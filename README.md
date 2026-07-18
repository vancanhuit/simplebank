# SimpleBank

A cloud-native banking service in Go: user accounts, JWT authentication, and
atomic money transfers between accounts, with asynchronous email verification.
Built on echo (HTTP), pgx + sqlc (PostgreSQL), and River (background jobs).

## Web UI

SimpleBank ships a Svelte 5 single-page app, built into `frontend/dist` and
embedded in the Go binary, so the API and UI are served from the same origin.
It covers the full journey: registration with email verification, account
creation with an optional opening balance, transfers between accounts, and
per-account transfer history.

| | |
|---|---|
| ![Sign in](docs/images/login.png) | ![Account overview](docs/images/dashboard.png) |
| **Sign in** | **Overview** — per-currency balances and accounts |
| ![Send money](docs/images/transfer.png) | ![Account activity](docs/images/account-activity.png) |
| **Send money** to another account | **Activity** — per-account transfer history |

## Quick Start

Prerequisites: [mise](https://mise.jdx.dev/) (manages the Go, golangci-lint, and
cocogitto toolchain) and Docker (for PostgreSQL and Mailpit).

```sh
mise install              # install pinned tools (Go 1.26.5, etc.)
mise run compose:dev:up   # start PostgreSQL + Mailpit + pgAdmin + app + worker (profile: dev)
```

The API and web UI are served at http://localhost:8080. Mailpit's web UI (sent
emails) is at http://localhost:8025, and pgAdmin (pre-wired to the dev database)
is at http://localhost:5050.

To run the server directly against your own database instead of the dev stack:

```sh
export DB_SOURCE="postgres://user:pass@localhost:5432/simplebank?sslmode=disable"
export JWT_SECRET="a-secret-at-least-32-characters-long"
export SMTP_FROM="no-reply@simplebank.local"
mise run app -- serve     # HTTP API
mise run app -- worker    # background worker
```

Migrations (schema + River) run automatically on startup.

## Commands

| Command | Description |
|---------|-------------|
| `mise run app` | Run the CLI (`serve` or `worker` subcommand) |
| `mise run app:build` | Build the single binary to `dist/simplebank`, with the SPA embedded |
| `mise run frontend:dev` | Run the Vite dev server (proxies `/api` to `:8080`) |
| `mise run frontend:build` | Build the SPA into `frontend/dist` |
| `mise run frontend:test` | Frontend unit tests (Vitest) |
| `mise run compose:dev:up` / `:down` | Start / stop the dev stack (DB, Mailpit, app) |
| `mise run compose:test:up` / `:down` | Start / stop the test stack |
| `mise run test:unit` | Unit tests (`-race -cover`) |
| `mise run test:integration` | Integration tests against a real PostgreSQL (`-tags=integration`) |
| `mise run golangci-lint` | Lint |
| `mise run golangci-lint:fmt` | Format |
| `mise run govulncheck` | Scan for known vulnerabilities |
| `mise run sqlc:generate` | Regenerate Go query code from `internal/db/query/*.sql` |
| `mise run docker:build` | Build a multi-arch Docker image |

## Configuration

All settings are supplied as CLI flags or environment variables (env var in
parentheses). Required: `DB_SOURCE`, `JWT_SECRET` (≥32 chars), `SMTP_FROM`.

| Env var | Flag | Default | Notes |
|---------|------|---------|-------|
| `HTTP_ADDR` | `--http-addr` | `:8080` | HTTP listen address |
| `DB_SOURCE` | `--db-source` | — | PostgreSQL DSN (required) |
| `DB_MAX_CONNS` | `--db-max-conns` | — | Max pool connections (0 = pgxpool/DSN default) |
| `DB_MIN_CONNS` | `--db-min-conns` | — | Min idle pool connections (0 = pgxpool/DSN default) |
| `JWT_SECRET` | `--jwt-secret` | — | HS256 signing key, ≥32 chars (required) |
| `ACCESS_TTL` | `--access-ttl` | `15m` | Access-token lifetime |
| `REFRESH_TTL` | `--refresh-ttl` | `24h` | Refresh-token lifetime |
| `SMTP_HOST` | `--smtp-host` | — | Mail server host |
| `SMTP_PORT` | `--smtp-port` | `1025` | Mail server port |
| `SMTP_USERNAME` | `--smtp-username` | — | Mail auth (optional) |
| `SMTP_PASSWORD` | `--smtp-password` | — | Mail auth (optional) |
| `SMTP_FROM` | `--smtp-from` | — | Sender address (required) |
| `RIVER_MAX_WORKERS` | `--river-max-workers` | `10` | Background worker concurrency |

## API

Base path `/api/v1`. Health endpoints (`/livez`, `/readyz`) are unversioned.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/livez` | — | Liveness (process up) |
| `GET` | `/readyz` | — | Readiness (database reachable) |
| `POST` | `/api/v1/users` | — | Register a user (queues a verification email) |
| `POST` | `/api/v1/users/login` | — | Log in, receive access + refresh tokens |
| `POST` | `/api/v1/tokens/renew` | — | Exchange a refresh token for a new access token |
| `GET` | `/api/v1/users/verify_email` | — | Verify an email via the link's `id` + `code` |
| `POST` | `/api/v1/accounts` | Bearer | Create an account (optional opening balance) |
| `GET` | `/api/v1/accounts/:id` | Bearer | Get an owned account |
| `GET` | `/api/v1/accounts` | Bearer | List owned accounts (paginated) |
| `GET` | `/api/v1/accounts/:id/transfers` | Bearer | List an owned account's transfer history (paginated) |
| `POST` | `/api/v1/transfers` | Bearer | Transfer between accounts you own |

Authenticated routes expect an `Authorization: Bearer <access-token>` header.

## Architecture

```
cmd/app/          CLI entrypoint; buildApp assembles shared dependencies
internal/
  api/            echo HTTP server, handlers, middleware, error mapping
  config/         flag/env configuration and validation
  currency/       supported-currency constants and validation
  db/             store seam: sqlc-generated queries + hand-written *Tx methods
    query/        SQL sources for sqlc
    sqlc/         generated Go (do not edit by hand)
    migrations/   goose schema migrations
  mail/           Mailer interface + SMTP adapter
  password/       bcrypt password hashing and verification
  random/         non-crypto random strings (test fixtures, display names)
  secret/         crypto-secure token generation
  token/          JWT Maker interface + HS256 implementation
  worker/         River job definitions (async verification email)
```

Key design decisions are recorded as ADRs in [docs/decisions/](docs/decisions/README.md):

- [ADR-0001](docs/decisions/0001-wide-sqlc-backed-store-interface.md) — the wide sqlc-backed `Store` interface.
- [ADR-0002](docs/decisions/0002-hash-refresh-tokens-at-rest.md) — hashing refresh tokens at rest.
- [ADR-0003](docs/decisions/0003-server-owns-routing-with-injected-readiness.md) — the Server owns routing with an injected readiness probe.
- [ADR-0004](docs/decisions/0004-split-util-into-domain-packages.md) — splitting `util` into domain packages, separating crypto from non-crypto randomness.

Money transfers run in a single database transaction with a deterministic
account lock order to avoid deadlocks; see `TransferTx` in
`internal/db/transfer_tx.go`.

## Testing

Unit tests run without external services. Integration tests are gated behind the
`integration` build tag and run against a real PostgreSQL brought up by the test
compose stack — `mise run test:integration` handles the lifecycle automatically.

## Contributing

Commits follow [Conventional Commits](https://www.conventionalcommits.org/),
enforced by cocogitto. Install the git hooks with `mise run hooks:install`.
Run `mise run golangci-lint` and `mise run test:unit` before opening a PR.
