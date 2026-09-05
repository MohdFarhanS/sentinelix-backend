# SentinelIX — Backend

**API Observability & Incident Management Platform** — a mini Sentry + Better Uptime built to
help developers detect application errors and endpoint downtime in real time, with automatic
notifications.

This is the backend service, written in Go with a strict clean architecture (domain → usecase →
repository → delivery). See [`sentinelix-frontend`](https://github.com/MohdFarhanS/sentinelix-frontend)
for the Next.js dashboard.

**Live:**
- Dashboard: https://sentinelix-frontend.vercel.app
- Status page API (try `/healthz` or `/api/v1/status/:slug`): https://sentinelix-status-api.onrender.com

## Features

- **Error ingestion & grouping** — errors are ingested via API key, fingerprinted from their
  stack trace, and grouped into issues instead of flooding the dashboard with duplicates.
- **Realtime dashboard updates** — issue and monitor status changes are pushed to connected
  clients over WebSocket.
- **Alerting** — configurable alert rules (new issue / threshold-based) with email (Resend) and
  Slack webhook notifications, including cooldown-based idempotency.
- **Uptime monitoring** — register any URL to be pinged on a custom interval; a
  goroutine-per-monitor scheduler tracks consecutive failures and triggers "down" notifications
  through a dedicated notifier interface.
- **Public status page** — a fully isolated, read-only service (`cmd/status-api`) serves
  aggregate uptime status per project, with no dependency on the dashboard's auth, WebSocket, or
  business logic — see [Isolated Status API](#isolated-status-api-cmdstatus-api) below.
- **Security hardening** — sliding-window rate limiting, rotating refresh tokens with theft
  detection, audit logging, and strict payload validation — see [Security](#security) below.
- **Reliability** — retry + dead-letter queue for the ingestion pipeline, a cached fallback for
  the API-key lookup, and fail-open rate limiting so infrastructure hiccups don't silently drop
  real error reports — see [Reliability](#reliability) below.

## Tech Stack

| Layer | Choice |
|---|---|
| Language | Go 1.25+ |
| HTTP router | [chi](https://github.com/go-chi/chi) |
| Database | PostgreSQL ([pgx/v5](https://github.com/jackc/pgx), pooled via `pgxpool`) — [Neon](https://neon.com) in production |
| Cache / Queue | Redis ([go-redis/v9](https://github.com/redis/go-redis)) — Streams for ingestion, Pub/Sub for realtime sync, sliding-window rate limiting — [Redis Cloud](https://redis.io/cloud) in production |
| Auth | Custom JWT (15-min access token) + opaque refresh token (30-day, hashed, rotated on every use) |
| Realtime | Native WebSocket ([gorilla/websocket](https://github.com/gorilla/websocket)) |
| Logging | [zerolog](https://github.com/rs/zerolog) (structured, with request-id) |
| Migrations | [golang-migrate](https://github.com/golang-migrate/migrate) |
| Notifications | [Resend](https://resend.com) (email), Slack Incoming Webhook |
| Containerization | Docker (multi-stage builds, one `Dockerfile.*` per binary) |
| Hosting | [Render](https://render.com) (3 free Web Services, Docker-based) |
| Testing | Go `testing` + [testify/mock](https://github.com/stretchr/testify) (unit) + [testcontainers-go](https://golang.testcontainers.org/) (repository) |
| Load testing | [k6](https://k6.io/) |

## Architecture

Clean architecture, dependencies point inward — `domain` has zero framework/DB knowledge:

```
delivery (HTTP handler, WS handler)
      ↓
usecase (business logic)
      ↓
domain (entities, repository interfaces)
      ↑
repository (Postgres / Redis implementations)
```
Three separate binaries share the same domain/usecase/repository layers, each with its own
`Dockerfile.*` and deployed as an independent Render Web Service:

- `cmd/api` (`Dockerfile.api`) — HTTP + WebSocket server (dashboard, ingestion endpoint, CRUD)
- `cmd/worker` (`Dockerfile.worker`) — background processing (ingestion consumer, alert
  evaluation, uptime checker); also exposes a minimal `/healthz` HTTP endpoint purely so Render's
  free tier (which only offers a "Web Service" instance type, not a free background-worker type)
  accepts it as a deployable service — the endpoint has no business logic
- `cmd/status-api` (`Dockerfile.status-api`) — public, read-only status page API, deployed as an
  **independent service** (see below)

### Isolated status API (`cmd/status-api`)

The public status page has a dedicated availability requirement: it must stay reachable even if
the dashboard backend (`cmd/api`) is down. A route inside `cmd/api` would share its fate — if
that process crashes, the status page dies with it.

`cmd/status-api` is a **separate Go binary and deployment unit** that:

- Only performs read-only `SELECT` queries — never writes.
- Has **zero import dependency** on the dashboard's `usecase` package, `AuthMiddleware`,
  `jwt.Manager`, `WSHandler`, or Redis pub/sub — so a bug anywhere in those code paths cannot
  affect this binary's compiled dependency graph, not just its runtime process.
- Runs its own `chi.NewRouter()` (`internal/delivery/router_status.go`), never reusing the
  dashboard router.

All three Render services are kept warm against Render's 15-minute idle spin-down by a single
GitHub Actions workflow (`.github/workflows/keep-warm.yml`), which pings each service's
`/healthz` every 10 minutes — deliberately **without** touching the database, so the keep-warm
ping doesn't also prevent Neon's Postgres compute from suspending during genuinely idle periods
(see below).

### Managing Neon's free-tier compute budget (100 CU-hrs/month)

`cmd/worker` needs to run continuously — this is a real production error tracker for an external
site, not just a demo, so alerts have to fire whenever something actually breaks, not only when
someone happens to be looking at the dashboard. That constraint is in direct tension with Neon's
free tier: at the smallest compute size (0.25 CU), running 24/7 for a month costs ~182 CU-hrs —
already over the 100 CU-hrs/month budget on its own, before counting any real traffic.

Two mitigations bring this back under budget, both aimed at giving Neon's ~5-minute idle
auto-suspend window an actual chance to trigger, rather than eliminating background work outright:

- **`alert_notifier` ticker widened from 1 minute to 10 minutes** (`cmd/worker/main.go`) — this
  only affects `condition_type="threshold"` alerts (periodic, sustained-count detection over a
  time window). `condition_type="new_issue"` alerts are unaffected — they're evaluated
  synchronously inside `ingest_consumer.go` the moment a new issue is created, not on this ticker.
  The PRD's <60s error-to-notification target is measured on that instant path, not this one.
- **`CachedProjectRepository`** (`internal/repository/redis/cached_project_repo.go`) — the
  API-key → project lookup on the ingestion hot path is cached in Redis (5-minute TTL) instead of
  hitting Postgres on every single ingested event.

Uptime monitors still tick at their own configured `interval_sec` (minimum 60s) independently of
these two changes — a project with several monitors on short intervals can still keep Neon mostly
awake. That trade-off is intentionally left to whoever configures the monitors, not enforced in
code.

## Security

Hardened in a dedicated sprint (full checklist in `06-ROADMAP.md` §6):

- **Rate limiting** — a sliding window counter (not fixed window, which allows up to 2x the
  configured limit to slip through at a window boundary), implemented once
  (`internal/repository/redis/ratelimiter.go`) and reused with different configuration for every
  use case: 100/min per API key for event ingestion, 10/15min per IP + 5/15min per email for
  login, and a generic 300/min per user across all other authenticated dashboard endpoints.
- **Refresh tokens** — access tokens are short-lived (15-min JWT); a separate opaque refresh
  token (30-day, SHA-256 hashed at rest, never a JWT) enables silent re-authentication. Refresh
  tokens rotate on every use, and reuse of an already-rotated token is treated as a theft signal
  — it revokes every active session for that user, not just the reused token.
- **Cross-site cookies** — the dashboard (Vercel) and API (Render) live on different registrable
  domains in production, a genuine cross-site context from the browser's point of view.
  `access_token`/`refresh_token` cookies use `SameSite=None; Secure` in production (derived from
  the same flag, `cfg.Env == "production"`) — `SameSite=Lax` is silently never sent by browsers on
  cross-site `fetch`/XHR calls, only on full-page navigation, which isn't how a SPA dashboard talks
  to its API.
- **Audit logging** — alert rule changes and project/API-key creation are recorded to a generic,
  polymorphic `audit_logs` table. Writes are best-effort: a logging failure is captured with
  structured fields (`action`, `resource_id`, `actor_user_id`, the underlying error) but never
  blocks the operation it's auditing.
- **Payload validation** — ingestion request bodies are capped at 256KB
  (`http.MaxBytesReader`), with additional field-level limits on message/stack-trace length and
  JSON context size.

## Reliability

Added after a resilience audit prompted by two questions: what happens to this service under
real infrastructure limits (Neon's free compute cap, Redis's free tier), and what happens if a
single point of failure loses an error report that a client is depending on?

- **Retry + dead-letter queue for ingestion** (`internal/worker/ingest_consumer.go`) — satisfies
  NFR-3. Structurally invalid messages (malformed JSON) are moved straight to a `queue:ingest:dlq`
  Redis Stream, since retrying them can never succeed. Messages that fail for likely-transient
  reasons (e.g. `groupIssue.Execute` failing because Postgres is temporarily unreachable) are left
  unacknowledged and picked up again by a separate `RunReclaim` goroutine, which uses Redis
  Streams' native `XPendingExt`/`XClaim` — including its built-in per-message delivery count — to
  retry up to a configurable limit before finally moving the message to the DLQ.
- **Fail-open rate limiting on ingestion** (`internal/usecase/ingest_event.go`) — if the Redis
  rate limiter itself errors (rather than legitimately rejecting the request), the event is still
  accepted rather than rejected with a `500`. Losing a real error report because the
  rate-limiting infrastructure had a hiccup is worse than a brief window without throttling; the
  request is still fully protected by API-key validation regardless of the rate limit outcome.
- **Cached project lookup as a fallback, not just an optimization** — see
  [Managing Neon's free-tier compute budget](#managing-neons-free-tier-compute-budget-100-cu-hrsmonth)
  above. The same cache that reduces Postgres load also means a project that ingested recently
  can keep ingesting for a few minutes even if Postgres is briefly unreachable.

## Getting Started

### Prerequisites

- Go 1.25+
- Docker & Docker Compose (for local Postgres + Redis, **and** for repository-level tests via
  testcontainers — see [Running Tests](#running-tests))
- [golang-migrate CLI](https://github.com/golang-migrate/migrate?tab=readme-ov-file#cli-usage)
- [k6](https://k6.io/docs/get-started/installation/) — optional, only needed for load testing

### 1. Clone and configure environment

```bash
git clone <this-repo-url>
cd sentinelix-backend
cp .env.example .env
```

`config.Load()` is shared by all three binaries, so `DATABASE_URL` and `JWT_SECRET` are required
even for binaries that don't use them directly (`cmd/status-api` never touches JWT, but still
needs a non-empty `JWT_SECRET` to pass the shared loader). `RESEND_API_KEY` is validated
separately, only inside `cmd/worker/main.go`, since it's the only binary that sends email.

| Variable | Required by | Notes |
|---|---|---|
| `DATABASE_URL` | all | Postgres connection string (pooled, in production) |
| `JWT_SECRET` | all (see above) | Only actually *used* by `cmd/api` |
| `REDIS_URL` | `cmd/api`, `cmd/worker` | Not used by `cmd/status-api` at all |
| `RESEND_API_KEY` | `cmd/worker` | Boots fine without it in `cmd/api`/`cmd/status-api` |
| `EMAIL_FROM_ADDRESS` | `cmd/worker` | Defaults to `onboarding@resend.dev` (Resend sandbox — can only deliver to the email the Resend account was created with, until a custom domain is verified) |
| `PORT` | `cmd/api` | Defaults to `8080`; Render injects this automatically in production |
| `APP_ENV` | all | `"development"` \| `"production"` — affects cookie `Secure`/`SameSite` |
| `FRONTEND_URL` | `cmd/api` | CORS whitelist — must exactly match the dashboard's origin, no trailing slash |

### 2. Start local infrastructure

```bash
docker compose up -d
```

This starts:
- PostgreSQL on `localhost:5433` (user/pass/db: `sentinelix`)
- Redis on `localhost:6379`

### 3. Run migrations

```bash
migrate -path migrations -database "$DATABASE_URL" up
```

Production migrations run the same way, but via a manual `workflow_dispatch` GitHub Actions
workflow (`.github/workflows/migrate.yml`) instead of a local CLI invocation — deliberately not
automatic on every push, and gated behind the `production` GitHub Environment for a manual
approval step. The workflow defaults to a `check`-only mode (prints the current migration version
without applying anything) so a misconfigured secret is caught before anything runs against the
real database.

### 4. Run the services

In up to three separate terminals, depending on what you need running:

```bash
# API server (dashboard + ingestion) — required for the dashboard
go run ./cmd/api

# Worker (ingestion consumer, alert evaluator, uptime checker) — required for
# alerts/uptime checks to actually run; NOT required just to browse the dashboard
go run ./cmd/worker

# Public status page API — only required to preview /status/[slug] locally
go run ./cmd/status-api
```

The API server listens on `PORT` (default `8080`). `cmd/status-api` listens on
`STATUS_API_PORT` (default `8081`), and `cmd/worker`'s `/healthz` listens on `PORT` (default
`8082` locally) — three separate defaults so all three can run side by side without a port
conflict.

## Running Tests

```bash
go test ./...
```

- **Unit tests** (`internal/usecase`, `internal/domain`, `internal/delivery/http`) use mocked
  repositories (`testify/mock`) or `httptest`; fast, no external dependencies.
- **Repository-level tests** (`internal/repository/postgres`, `internal/repository/redis`,
  `internal/worker`) spin up real Postgres/Redis containers via testcontainers-go — a fresh
  container per test function for full isolation, torn down automatically. **Requires Docker
  running locally.** These caught two real bugs mocked unit tests couldn't have: a divide-by-zero
  panic in the rate limiter for sub-second windows, and a Windows-only bug in the migration
  loader (representing a local path as a `file://` URL broke on drive-letter paths — fixed by
  switching to `source/iofs` + `os.DirFS`, which never represents a local path as a URL string).

## Load Testing

```bash
k6 run loadtest/ingest_load_test.js
```

Validates NFR-1 (ingestion endpoint sustains 100 req/s on one instance) using 100 API keys
round-robined so no single key hits its own rate limit, plus a second scenario that deliberately
overloads one API key to confirm the rate limiter rejects excess traffic with `429`s instead of
degrading or crashing. Requires a running `cmd/api` instance with its own Postgres/Redis.

## Deployment

All three binaries run as separate free Render Web Services, built from their own `Dockerfile.*`
— Render's free tier only offers the "Web Service" instance type (must listen on HTTP), not a
free background-worker type, which is why `cmd/worker` carries the minimal `/healthz` endpoint
mentioned above.

| Service | Platform | Notes |
|---|---|---|
| `cmd/api` | Render (Docker) | `PORT` is auto-injected by Render; no manual override needed |
| `cmd/worker` | Render (Docker) | Needs `RESEND_API_KEY`; runs continuously (see [compute budget](#managing-neons-free-tier-compute-budget-100-cu-hrsmonth) above) |
| `cmd/status-api` | Render (Docker) | `PORT` must be set manually to match `STATUS_API_PORT` (the app reads `STATUS_API_PORT`, not Render's own `PORT`, internally) |
| Database | [Neon](https://neon.com) | Free tier, pooled connection string |
| Redis | [Redis Cloud](https://redis.io/cloud) | Free tier — a separate account from any Redis already used by other projects, since most providers cap free tier at one database per account |
| Email | [Resend](https://resend.com) | Free tier, sandbox sender (`onboarding@resend.dev`) until a custom domain is verified |
| Frontend | [Vercel](https://vercel.com) | See [`sentinelix-frontend`](https://github.com/MohdFarhanS/sentinelix-frontend) |

CI/CD: `ci.yml` (lint + test + build on every push), `migrate.yml` (manual production migrations,
`production` environment gate), `keep-warm.yml` (cron ping to all three Render services every 10
minutes).

## API Documentation

An OpenAPI 3.0 spec is planned to be generated and published as the API surface stabilizes; in
the meantime, `04-API-DESIGN.md` in the planning docs is the authoritative reference, including
rate limits and the refresh token flow.

## Project Status

Live and deployed. All 10 planned sprints complete: auth, project management, error ingestion &
grouping, realtime dashboard, alerting (email/Slack), uptime monitoring, a public status page
served by an isolated `cmd/status-api` service, a full security-hardening pass (rate limiting,
refresh tokens, audit logging, load testing), and a post-deploy resilience pass covering
production infrastructure limits (see [Reliability](#reliability) above).

Currently monitoring [newsportal.my.id](https://newsportal.my.id) in production.

## License

MIT