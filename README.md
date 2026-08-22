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
mise install              # install pinned tools (Go 1.26.6, Bun 1.3.14, etc.)
mise run compose:dev:up   # start PostgreSQL + Mailpit + pgAdmin + app (profile: dev)
```

The API and web UI are served at http://localhost:8080. Mailpit's web UI (sent
emails) is at http://localhost:8025, and pgAdmin (pre-wired to the dev database)
is at http://localhost:5050.

The dev stack runs Mailpit like a real provider: SMTP over implicit TLS (SSL) on
port 465 with username/password auth. The `compose:dev:up` task generates the
mkcert certificates it needs (`dev:tls:certs`); the app trusts the mkcert root
via `SMTP_TLS_CA_FILE` and authenticates with the throwaway credentials in
`mailpit/smtp-auth.txt`.

Two additional Compose profiles exercise production-style TLS topologies:

```sh
mise run compose:dev-https:up  # app terminates HTTPS directly at https://localhost:8443
mise run compose:dev-proxy:up  # Caddy terminates HTTPS and forwards trusted headers
```

Use the matching `:down` task to stop either profile and remove its volumes.

To run the server directly against your own database instead of the dev stack:

```sh
export DB_SOURCE="postgres://user:pass@localhost:5432/simplebank?sslmode=disable"
export JWT_SECRET="a-secret-at-least-32-characters-long"
export SMTP_FROM="no-reply@simplebank.local"
mise run app -- serve     # HTTP API and background worker
```

Migrations (schema + River) run automatically on startup.

## Commands

| Command | Description |
|---------|-------------|
| `mise run app` | Run the CLI (`serve`, `healthcheck`, or `version` subcommand) |
| `mise run app:build` | Build the single binary to `dist/simplebank`, with the SPA embedded |
| `mise run frontend:dev` | Run the Vite dev server (proxies `/api` to `:8080`) |
| `mise run frontend:preview` | Preview the production frontend build |
| `mise run frontend:build` | Build the SPA into `frontend/dist` |
| `mise run frontend:check` | Type-check the frontend with `svelte-check` and TypeScript |
| `mise run frontend:lint` | Lint the frontend with ESLint |
| `mise run frontend:format:check` | Check frontend formatting with Prettier |
| `mise run frontend:test` | Frontend unit tests (Vitest) |
| `mise run frontend:audit` | Audit frontend dependencies for known vulnerabilities |
| `mise run frontend:test:e2e` | Frontend responsive accessibility tests (Playwright + axe) |
| `mise run frontend:test:e2e:install` | Install Chromium required by frontend browser tests |
| `mise run compose:dev:up` / `:down` | Start / stop the dev stack (DB, Mailpit, app) |
| `mise run compose:dev-https:up` / `:down` | Start / stop the direct-HTTPS dev stack |
| `mise run compose:dev-proxy:up` / `:down` | Start / stop the Caddy reverse-proxy dev stack |
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
| `DB_MAX_CONN_LIFETIME` | `--db-max-conn-lifetime` | — | Max connection lifetime (0 = pgxpool/DSN default) |
| `DB_MAX_CONN_IDLE_TIME` | `--db-max-conn-idle-time` | — | Max connection idle time (0 = pgxpool/DSN default) |
| `JWT_SECRET` | `--jwt-secret` | — | HS256 signing key, ≥32 chars (required) |
| `ACCESS_TTL` | `--access-ttl` | `15m` | Access-token lifetime |
| `REFRESH_TTL` | `--refresh-ttl` | `24h` | Refresh-token lifetime |
| `SMTP_HOST` | `--smtp-host` | — | Mail server host |
| `SMTP_PORT` | `--smtp-port` | `1025` | Mail server port |
| `SMTP_USERNAME` | `--smtp-username` | — | Mail auth (optional) |
| `SMTP_PASSWORD` | `--smtp-password` | — | Mail auth (optional) |
| `SMTP_FROM` | `--smtp-from` | — | Sender address (required) |
| `SMTP_INSECURE` | `--smtp-insecure` | `false` | Disable TLS (plaintext SMTP) |
| `SMTP_SSL` | `--smtp-ssl` | `false` | Use implicit TLS (SSL, e.g. port 465) instead of STARTTLS |
| `SMTP_TLS_CA_FILE` | `--smtp-tls-ca-file` | — | PEM CA bundle to verify the mail server cert (e.g. a mkcert root) |
| `RIVER_MAX_WORKERS` | `--river-max-workers` | `10` | Background worker concurrency per API replica |
| `TLS_CERT_FILE` | `--tls-cert-file` | — | Serve HTTPS with this cert (must be set with the key) |
| `TLS_KEY_FILE` | `--tls-key-file` | — | Private key for `TLS_CERT_FILE` |
| `TRUSTED_PROXIES` | `--trusted-proxies` | — | CIDRs/IPs whose forwarded headers are trusted (repeatable) |
| `PUBLIC_BASE_URL` | `--public-base-url` | — | External base URL used to build email verification links |
| `SESSION_COOKIE_SECURE` | `--session-cookie-secure` | `true` | Require HTTPS for refresh cookie transport |
| `TRANSFER_LIMITS` | `--transfer-limits` | — | Per-currency ceilings as JSON; see [Transfer limits](#transfer-limits) |
| `ACCOUNT_OPENING_LIMITS` | `--account-opening-limits` | — | Per-currency demo opening caps in minor units |

## API

Base path `/api/v1`. Health endpoints (`/livez`, `/readyz`) are unversioned.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/livez` | — | Liveness (process up) |
| `GET` | `/readyz` | — | Readiness (database reachable) |
| `POST` | `/api/v1/users` | — | Register a user, return `202 Accepted`, and queue verification mail |
| `POST` | `/api/v1/users/login` | — | Log in, receive an access token, and set the refresh token as an HttpOnly cookie |
| `POST` | `/api/v1/users/logout` | Refresh cookie | Block the current session and clear the refresh cookie (`204 No Content`) |
| `POST` | `/api/v1/tokens/renew` | Refresh cookie | Rotate the refresh cookie and return a fresh access token |
| `POST` | `/api/v1/users/verify_email/resend` | — | Return `202 Accepted` and (re)queue verification mail without disclosing account existence |
| `GET` | `/api/v1/users/verify_email` | — | Verify an email via the link's `id` + `code` |
| `GET` | `/api/v1/transfer-limits` | — | Per-currency transfer ceilings (policy, so the UI validates against the same limits) |
| `GET` | `/api/v1/account-opening-limits` | — | Per-currency opening-balance caps for the account-opening form |
| `POST` | `/api/v1/accounts` | Bearer | Create an account (optional opening balance) |
| `GET` | `/api/v1/accounts/:id` | Bearer | Get an owned account |
| `GET` | `/api/v1/accounts` | Bearer | List owned accounts (paginated) |
| `GET` | `/api/v1/accounts/:id/transfers` | Bearer | List an owned account's transfer history (paginated) |
| `POST` | `/api/v1/transfers` | Bearer | Transfer from an account you own to another same-currency account (requires an `idempotency_key`) |

Protected routes accept only `Authorization: Bearer <access-token>` credentials.
Refresh tokens are never returned in JSON responses; the browser receives them
in the `simplebank_refresh` HttpOnly, `SameSite=Strict` cookie scoped to
`/api/v1`, with `Secure` enabled by default. Only the renew/logout endpoints
consume that cookie, and both reject cross-origin browser requests. Registration
requires a password of at least 15 characters and returns the same accepted
response when a username or email is already registered.

All `/api/v1` responses use `Cache-Control: no-store`. Transfer responses return
the transfer and the caller's updated source account, but do not expose a
recipient account snapshot.

### Transfer limits

`TRANSFER_LIMITS` is a JSON object keyed by currency code. Each entry sets a
`max_per_transfer` and an optional `daily` ceiling, both in that currency's own
minor units (USD/EUR cents, VND whole dong). A zero or omitted field disables
that limit, so limits are opt-in per currency, and a currency with no entry is
unlimited.

```sh
export TRANSFER_LIMITS='{"USD":{"max_per_transfer":100000,"daily":500000},"VND":{"max_per_transfer":2000000000}}'
```

Each transfer request carries a client-generated `idempotency_key` (a UUID) so a
retry after a lost response replays the original transfer instead of moving
money twice. See [ADR-0005](docs/decisions/0005-transfer-safety-idempotency-and-limits.md)
for the full rationale.

The API authorizes the source account before looking up the destination, so an
unauthorized caller cannot use transfer validation to probe whether another
account exists. A replay returns the same transfer but reconstructs account
snapshots from current balances, which may include later activity.

### Account opening limits

`ACCOUNT_OPENING_LIMITS` is a JSON object keyed by currency code. Each value is
the maximum opening balance the demo permits for a newly created account,
expressed in that currency's minor units. A missing currency entry means a zero
cap: the account may still be opened, but only with a zero opening deposit.

```sh
export ACCOUNT_OPENING_LIMITS='{"USD":100000,"EUR":100000,"VND":25000000}'
```

The SPA reads the live policy from `/api/v1/account-opening-limits` and validates
the requested opening deposit before submit, while the server remains
authoritative and enforces the same cap before account creation.

## Architecture

See the [documentation index](docs/README.md) for architecture decisions,
feature designs, and implementation-plan history.

```
caddy/            reverse-proxy configuration for the proxy-aware dev profile
cmd/app/          CLI entrypoint; buildApp assembles shared dependencies
frontend/         Svelte 5 SPA, tests, and Vite build embedded by the Go binary
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
  worker/         River client and async verification-email jobs
```

Key design decisions are recorded as ADRs in [docs/decisions/](docs/decisions/README.md):

- [ADR-0001](docs/decisions/0001-wide-sqlc-backed-store-interface.md) — the wide sqlc-backed `Store` interface.
- [ADR-0002](docs/decisions/0002-hash-refresh-tokens-at-rest.md) — hashing refresh tokens at rest.
- [ADR-0003](docs/decisions/0003-server-owns-routing-with-injected-readiness.md) — the Server owns routing with an injected readiness probe.
- [ADR-0004](docs/decisions/0004-split-util-into-domain-packages.md) — splitting `util` into domain packages, separating crypto from non-crypto randomness.
- [ADR-0005](docs/decisions/0005-transfer-safety-idempotency-and-limits.md) — idempotent transfers with source-first authorization, in-transaction re-validation and daily limits, plus API-edge per-transfer caps.
- [ADR-0006](docs/decisions/0006-run-worker-with-http-server.md) — running the River worker in the HTTP server process with ordered startup and shutdown.

Money transfers run in a single database transaction that locks both accounts in
a deterministic order (avoiding deadlocks), re-validates currency against the
locked rows (closing the TOCTOU gap), enforces rolling daily limits, and moves
balances under a guarded `UPDATE` that rejects overdrafts. The API enforces the
configured per-transfer cap before opening the transaction. A client-supplied
idempotency key collapses retries onto a single transfer. See `TransferTx` in
`internal/db/transfer_tx.go` and
[ADR-0005](docs/decisions/0005-transfer-safety-idempotency-and-limits.md).

The `serve` command starts River before accepting HTTP traffic and shuts HTTP
down before giving in-progress jobs a bounded grace period. Each API replica
runs its own River client, so `RIVER_MAX_WORKERS` is a per-replica concurrency
limit. See [ADR-0006](docs/decisions/0006-run-worker-with-http-server.md).

The SPA treats account data as session-scoped. Losing authentication clears the
account store, and generation checks discard load or create responses that were
started before the reset so stale data cannot cross sessions.

## Security

SimpleBank applies a restrictive Content Security Policy, anti-framing and
content-type protections, a no-referrer policy, HSTS, request size and timeout
limits, and per-client throttling on authentication endpoints. Access tokens are
short-lived and held in browser memory; refresh tokens and email verification
codes are stored as digests rather than reusable plaintext credentials.

Forwarded headers are ignored unless the connecting proxy is covered by an
explicit `TRUSTED_PROXIES` CIDR. Production deployments should terminate TLS,
leave `SESSION_COOKIE_SECURE=true`, set `PUBLIC_BASE_URL` to the external origin,
and configure only the proxy ranges that can connect directly to the app.

See the [security guide](docs/security.md) for the complete control inventory,
deployment checklist, migration impact, and known limitations.

## Testing

Unit tests run without external services. Integration tests are gated behind the
`integration` build tag and run against a real PostgreSQL brought up by the test
compose stack — `mise run test:integration` handles the lifecycle automatically.
The frontend has Vitest unit coverage plus Playwright checks across responsive
viewports with axe accessibility assertions. Run `mise run frontend:check`,
`mise run frontend:lint`, `mise run frontend:audit`, `mise run frontend:test`, and
`mise run frontend:test:e2e` before shipping UI changes.

## Contributing

Commits follow [Conventional Commits](https://www.conventionalcommits.org/),
enforced by cocogitto. Install the git hooks with `mise run hooks:install`.
The pre-commit hook rejects unformatted staged Go files and stale or unstaged
sqlc output when database sources change.
Run `mise run golangci-lint` and `mise run test:unit` before opening a PR.
