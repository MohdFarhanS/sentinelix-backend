# SentinelIX — Backend

**API Observability & Incident Management Platform** — a mini Sentry + Better Uptime built to
help developers detect application errors and endpoint downtime in real time, with automatic
notifications.

This is the backend service, written in Go with a strict clean architecture (domain → usecase →
repository → delivery). See [`sentinelix-frontend`](#) for the Next.js dashboard.

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

## Tech Stack

| Layer | Choice |
|---|---|
| Language | Go 1.22+ |
| HTTP router | [chi](https://github.com/go-chi/chi) |
| Database | PostgreSQL ([pgx/v5](https://github.com/jackc/pgx), pooled via `pgxpool`) |
| Cache / Queue | Redis ([go-redis/v9](https://github.com/redis/go-redis)) — Streams for ingestion, Pub/Sub for realtime sync, sliding-window rate limiting |
| Auth | Custom JWT (15-min access token) + opaque refresh token (30-day, hashed, rotated on every use) |
| Realtime | Native WebSocket ([gorilla/websocket](https://github.com/gorilla/websocket)) |
| Logging | [zerolog](https://github.com/rs/zerolog) (structured, with request-id) |
| Migrations | [golang-migrate](https://github.com/golang-migrate/migrate) |
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


Three separate binaries share the same domain/usecase/repository layers:

- `cmd/api` — HTTP + WebSocket server (dashboard, ingestion endpoint, CRUD)
- `cmd/worker` — background processing (ingestion consumer, alert evaluation, uptime checker)
- `cmd/status-api` — public, read-only status page API, deployed as an **independent service**
  (see below)

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

It's deployed as a free Render Web Service, kept warm against Render's 15-minute idle
spin-down via a scheduled `GET /healthz` ping (GitHub Actions cron) — deliberately **without**
touching the database, so the keep-warm ping doesn't also prevent Neon's Postgres compute from
suspending during genuinely idle periods (see below).

### A deliberate trade-off: Neon compute vs. monitoring precision

`cmd/worker` runs a goroutine-per-monitor scheduler (each monitor gets its own ticker at its
configured `interval_sec`, minimum 60s) plus a 1-minute alert-threshold ticker. This means the
database is touched at least every ~60 seconds whenever there's at least one active monitor or
alert rule — which prevents Neon's free-tier compute from ever reaching its ~5 minute idle
threshold for auto-suspend, regardless of actual traffic.

This is a known, accepted limitation: `cmd/worker` is designed for monitoring precision, not for
minimizing idle compute cost. It's intended to run during active development/demos, not as an
always-on 24/7 process on the free tier. `cmd/status-api`, by contrast, is explicitly configured
with `pgxpool.MinConns = 0` and a short `MaxConnIdleTime` so its own connection pool never holds
Neon awake — its compute usage tracks real visitor traffic, not the mere fact that the service is
deployed.

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
- **Audit logging** — alert rule changes and project/API-key creation are recorded to a generic,
  polymorphic `audit_logs` table. Writes are best-effort: a logging failure is captured with
  structured fields (`action`, `resource_id`, `actor_user_id`, the underlying error) but never
  blocks the operation it's auditing.
- **Payload validation** — ingestion request bodies are capped at 256KB
  (`http.MaxBytesReader`), with additional field-level limits on message/stack-trace length and
  JSON context size.

## Getting Started

### Prerequisites

- Go 1.22+
- Docker & Docker Compose (for local Postgres + Redis, **and** for repository-level tests via
  testcontainers — see [Running Tests](#running-tests))
- [golang-migrate CLI](https://github.com/golang-migrate/migrate?tab=readme-ov-file#cli-usage)
- [k6](https://k6.io/docs/get-started/installation/) — optional, only needed for load testing

### 1. Clone and configure environment

```bash
git clone <this-repo-url>
cd sentinelix-backend
cp .env.example .env
# edit .env — fill in JWT_SECRET, RESEND_API_KEY, STATUS_API_PORT, etc.
```

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
`STATUS_API_PORT` (default `8081`) — a separate variable so it can run alongside `cmd/api`
without a port conflict.

## Running Tests

```bash
go test ./...
```

- **Unit tests** (`internal/usecase`, `internal/domain`, `internal/delivery/http`) use mocked
  repositories (`testify/mock`) or `httptest`; fast, no external dependencies.
- **Repository-level tests** (`internal/repository/postgres`, `internal/repository/redis`) spin
  up real Postgres/Redis containers via testcontainers-go — a fresh container per test function
  for full isolation, torn down automatically. **Requires Docker running locally.** These caught
  a real bug (a divide-by-zero panic in the rate limiter for sub-second windows) that mocked
  unit tests couldn't have found.

## Load Testing

```bash
k6 run loadtest/ingest_load_test.js
```

Validates NFR-1 (ingestion endpoint sustains 100 req/s on one instance) using 100 API keys
round-robined so no single key hits its own rate limit, plus a second scenario that deliberately
overloads one API key to confirm the rate limiter rejects excess traffic with `429`s instead of
degrading or crashing. Requires a running `cmd/api` instance with its own Postgres/Redis.

## API Documentation

An OpenAPI 3.0 spec is planned to be generated and published as the API surface stabilizes; in
the meantime, `04-API-DESIGN.md` in the planning docs is the authoritative reference, including
rate limits and the refresh token flow.

## Project Status

Actively developed as a portfolio project. Sprints 1–9 complete: auth, project management, error
ingestion & grouping, realtime dashboard, alerting (email/Slack), uptime monitoring, a public
status page served by an isolated `cmd/status-api` service, and a full security-hardening pass
(rate limiting, refresh tokens, audit logging, load testing — see [Security](#security)).
Deployment (Render + GitHub Actions keep-warm cron) is the only remaining item before launch.

## License

MIT