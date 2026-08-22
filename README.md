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

## Tech Stack

| Layer | Choice |
|---|---|
| Language | Go 1.22+ |
| HTTP router | [chi](https://github.com/go-chi/chi) |
| Database | PostgreSQL ([pgx/v5](https://github.com/jackc/pgx), pooled via `pgxpool`) |
| Cache / Queue | Redis ([go-redis/v9](https://github.com/redis/go-redis)) — Streams for ingestion, Pub/Sub for realtime sync |
| Realtime | Native WebSocket ([gorilla/websocket](https://github.com/gorilla/websocket)) |
| Logging | [zerolog](https://github.com/rs/zerolog) (structured, with request-id) |
| Migrations | [golang-migrate](https://github.com/golang-migrate/migrate) |
| Testing | Go `testing` + [testify/mock](https://github.com/stretchr/testify) |

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

## Getting Started

### Prerequisites

- Go 1.22+
- Docker & Docker Compose (for local Postgres + Redis)
- [golang-migrate CLI](https://github.com/golang-migrate/migrate?tab=readme-ov-file#cli-usage)

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

Unit tests cover the usecase layer using mocked repositories (`testify/mock`); HTTP-dependent
logic (e.g. uptime pings) is tested against `httptest.Server`.

## API Documentation

An OpenAPI 3.0 spec is planned to be generated and published as the API surface stabilizes.

## Project Status

Actively developed as a portfolio project. Core feature set complete: auth, project management,
error ingestion & grouping, realtime dashboard, alerting (email/Slack), uptime monitoring, and a
public status page served by an isolated `cmd/status-api` service. Deployment (Render + GitHub
Actions keep-warm cron) pending — all sprints are being finished before a single simultaneous
deploy.

## License

MIT