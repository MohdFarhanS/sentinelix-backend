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

Two separate binaries share the same domain/usecase/repository layers:

- `cmd/api` — HTTP + WebSocket server (dashboard, ingestion endpoint, CRUD)
- `cmd/worker` — background processing (ingestion consumer, alert evaluation, uptime checker)

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
# edit .env — fill in JWT_SECRET, RESEND_API_KEY, etc.
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

In two separate terminals:

```bash
# API server
go run ./cmd/api

# Worker (ingestion consumer, alert evaluator, uptime checker)
go run ./cmd/worker
```

The API server listens on `PORT` (default `8080`).

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
error ingestion & grouping, realtime dashboard, alerting (email/Slack), and uptime monitoring
with a public status page in progress.

## License

MIT