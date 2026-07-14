# SimpleBank — Cloud-Native Design Spec

Date: 2026-07-14
Status: Approved (pending implementation plan)

A production-grade, intentionally-simple reimplementation of TechSchool's
[simplebank](https://github.com/techschool/simplebank) using a modern Go stack
and cloud-native patterns.

## Goals

- Reproduce the core banking domain and the key concurrency lesson (atomic money
  transfers) from the tutorial.
- Use a modern, Postgres-centric Go stack aligned with current industry practice.
- Stay "simple enough" while following production practices and security hardening.

## Non-Goals

- gRPC / protobuf (REST only).
- Kubernetes manifests, service mesh, metrics/tracing backends (container-only ops).
- Multi-region, sharding, or advanced financial features.

## Stack Decisions

| Area | Choice | Notes |
|------|--------|-------|
| Language | Go 1.26 | Already scaffolded |
| HTTP framework | Echo v5 | Already scaffolded |
| DB | PostgreSQL 18 | Native `uuidv7()` |
| DB driver | pgx/v5 (`pgxpool`) | |
| Query codegen | sqlc | Generates `Querier` bound to `DBTX` |
| Domain migrations | goose | Embedded via `embed.FS` |
| Queue + queue migrations | River | Postgres-backed; `rivermigrate` owns its tables |
| Auth | JWT via `echo-jwt` | Access + refresh tokens |
| Async | River worker | Email verification |
| Mail | `github.com/wneessen/go-mail` | Generic SMTP, provider-agnostic |
| Config + CLI | urfave/cli v3 | Flags with env value-sources |
| Password hashing | bcrypt (cost ≥ 12) | |
| Rate limiting | `golang.org/x/time/rate` | Auth endpoints |
| Logging | `log/slog` | JSON to stdout |
| Lint / vuln | golangci-lint, govulncheck | Already wired in `mise.toml` |
| Containers | Distroless multi-arch | Already scaffolded |

Primary keys: **UUID v7 for all tables** via Postgres 18 native `uuidv7()`.
Money: **`bigint` minor units (cents)** — never floats.

## Architecture

Single binary, three subcommands (urfave/cli v3):

- `serve` — Echo HTTP API.
- `migrate` — run goose (domain) + River migrations, then exit.
- `worker` — River worker process (email jobs).

### Package layout

```
cmd/app/main.go            # urfave/cli v3 root, wires subcommands
internal/
  config/                  # Config struct from cli flags/env value-sources; validated
  db/
    migrations/            # goose *.sql, embedded via embed.FS
    query/                 # sqlc *.sql source
    sqlc/                  # generated: models, Querier, Queries
    store.go               # Store iface + execTx + TransferTx
  api/                     # Echo server, handlers, routes, middleware, DTOs
  token/                   # JWT maker (create/verify access + refresh)
  worker/                  # River client, workers, job args
  mail/                    # Mailer iface + SMTP (go-mail) impl
  util/                    # password hash, random, currency validation
sqlc.yaml
```

**Deep-module boundaries:** each `internal/*` package has one purpose, exposes an
interface, hides internals. `api` depends on `db.Store`, `token.Maker`,
`worker.Client`, `mail.Mailer` — all interfaces, each testable in isolation.
Constructor-based dependency injection throughout.

## Data Model

All PKs are `uuid PRIMARY KEY DEFAULT uuidv7()`. Timestamps `timestamptz`.

```
users
  id                 uuid PK
  username           text UNIQUE NOT NULL
  hashed_password    text NOT NULL
  full_name          text NOT NULL
  email              text UNIQUE NOT NULL
  is_email_verified  bool NOT NULL DEFAULT false
  password_changed_at timestamptz NOT NULL DEFAULT '0001-01-01'
  created_at         timestamptz NOT NULL DEFAULT now()

verify_emails
  id          uuid PK
  username    text NOT NULL REFERENCES users(username)
  email       text NOT NULL
  secret_code text NOT NULL
  is_used     bool NOT NULL DEFAULT false
  created_at  timestamptz NOT NULL DEFAULT now()
  expired_at  timestamptz NOT NULL DEFAULT now() + interval '15 minutes'

accounts
  id         uuid PK
  owner      text NOT NULL REFERENCES users(username)
  balance    bigint NOT NULL
  currency   text NOT NULL
  created_at timestamptz NOT NULL DEFAULT now()
  UNIQUE(owner, currency)

entries
  id         uuid PK
  account_id uuid NOT NULL REFERENCES accounts(id)
  amount     bigint NOT NULL           -- signed
  created_at timestamptz NOT NULL DEFAULT now()

transfers
  id              uuid PK
  from_account_id uuid NOT NULL REFERENCES accounts(id)
  to_account_id   uuid NOT NULL REFERENCES accounts(id)
  amount          bigint NOT NULL CHECK (amount > 0)
  created_at      timestamptz NOT NULL DEFAULT now()

sessions
  id            uuid PK
  username      text NOT NULL REFERENCES users(username)
  refresh_token text NOT NULL
  user_agent    text NOT NULL
  client_ip     text NOT NULL
  is_blocked    bool NOT NULL DEFAULT false
  expires_at    timestamptz NOT NULL
  created_at    timestamptz NOT NULL DEFAULT now()
```

Indexes: `accounts(owner)`, `entries(account_id)`, `transfers(from_account_id)`,
`transfers(to_account_id)`. River tables live in a separate migration namespace via
`rivermigrate`, untouched by goose.

## Transfer Transaction (core concurrency lesson)

**Invariant:** a transfer atomically moves `amount` from account A to B; total
balance conserved; no balance goes negative; two `entries` + two balance updates +
one `transfers` row all commit together or none.

**Unsafe interleaving:** concurrent transfers on the same accounts cause lost
updates or deadlocks if locks are acquired in inconsistent order.

**Boundary:** one `execTx` at Read Committed. `Store.TransferTx(ctx, arg)`:

1. Insert `transfers` row.
2. Insert two `entries` (−amount from, +amount to).
3. Update balances with an **atomic conditional UPDATE** (no read-modify-write):

   ```sql
   UPDATE accounts SET balance = balance - $1
   WHERE id = $2 AND balance >= $1
   RETURNING *;
   ```

   Zero rows affected → `ErrInsufficientBalance`.
4. **Deterministic lock order:** always update the smaller `account_id` first
   (compare UUIDs) so both directions lock in the same order — prevents deadlocks.

**Retry:** `execTx` wrapper retries the whole callback on SQLSTATE `40001` /
`40P01`, bounded by attempts + parent context, with jittered backoff. The callback
is pure DB — no email or external side effects inside the transaction.

**Error mapping:** pgx errors are translated to stable app errors at the store
boundary (`ErrUniqueViolation`, `ErrForeignKeyViolation`, `ErrInsufficientBalance`,
`ErrNotFound`) and mapped to HTTP status codes in `api`. `pgx.Tx` never leaves the
store; sqlc queries are rebound with `Queries.WithTx(tx)`.

**Concurrency test:** N concurrent goroutines run transfers in both directions;
assert balances conserved, no deadlock, and per-account balance changes are
consistent.

## API

JSON APIs under `/api/v1`. Health probes at root.

### Public

- `POST /api/v1/users` — register; bcrypt hash; enqueue `SendVerifyEmail` River job
  in the same tx → 201.
- `POST /api/v1/users/login` — verify password; issue access + refresh tokens;
  create `sessions` row → 200.
- `POST /api/v1/tokens/renew` — validate refresh token + session (not blocked, not
  expired) → new access token.
- `GET /api/v1/users/verify_email?id=&code=` — mark `verify_emails.is_used`; set
  `users.is_email_verified`.

### Protected (`echo-jwt`, Bearer access token)

- `POST /api/v1/accounts` — create account for authed owner (unique owner+currency).
- `GET /api/v1/accounts/:id` — owner-only (403 if not owner).
- `GET /api/v1/accounts?page=&size=` — list caller's accounts, paginated.
- `POST /api/v1/transfers` — from-account must belong to caller; both accounts same
  currency → `TransferTx`.

### Health (no auth)

- `GET /livez` — process alive, **no DB dependency** (liveness).
- `GET /readyz` — `pool.Ping(ctx)` with short deadline (readiness).

### Tokens

Access token short TTL (15m); refresh long TTL (24h) stored in `sessions`. JWT
claims: `sub`=username, `jti`=uuid, `exp`, `iat`, `role`. `echo-jwt` middleware
injects claims into `echo.Context`; handlers perform owner authorization.

Handlers stay thin: parse/validate DTO, call store/service, map errors → HTTP. No
pgx types leak into `api`.

## Async Email Flow (River)

**Enqueue:** `POST /api/v1/users` inserts the user and enqueues
`SendVerifyEmailArgs{Username}` in the **same pgx transaction** via River's
transactional insert — job and user commit atomically (transactional-outbox
property; no orphaned or lost jobs).

**Worker (`app worker`):** River polls Postgres and runs `SendVerifyEmailWorker`:

1. Load user; create `verify_emails` row (secret code, 15m expiry).
2. Build verification link; send via `mail.Mailer` (go-mail SMTP → Mailpit locally).
3. Idempotent: reuse a valid unused code if present; SMTP send is safe to retry.
   River retries with backoff; poison jobs go to River's dead-letter after max
   attempts.

**Consistency:** the DB transaction covers user + job insert only. SMTP is a
separate system — correctness is preserved by worker retry + idempotency, **not**
claimed atomic with the database.

**Config:** River queue name, max workers, and retry policy in `config`. Worker
shares the pgx pool and drains in-flight jobs on SIGTERM.

## Config / CLI (urfave/cli v3)

Root command with global flags, each backed by an env value-source:

```
--http-addr         (HTTP_ADDR, :8080)
--db-source         (DB_SOURCE)              # pgx pool DSN
--jwt-secret        (JWT_SECRET)             # required, min length validated
--access-ttl        (ACCESS_TTL, 15m)
--refresh-ttl       (REFRESH_TTL, 24h)
--smtp-host         (SMTP_HOST)
--smtp-port         (SMTP_PORT)
--smtp-username     (SMTP_USERNAME)
--smtp-password     (SMTP_PASSWORD)
--smtp-from         (SMTP_FROM)
--river-max-workers (RIVER_MAX_WORKERS, 10)
```

Subcommands: `serve`, `migrate`, `worker`. Config validated at startup; fail fast on
missing/invalid values.

## Error Handling

Stable app error sentinels classified via `errors.Is` / `errors.As`. A centralized
Echo `HTTPErrorHandler` maps categories → 400 / 401 / 403 / 404 / 409 / 422 / 500.
Internal details never leak to clients; full detail logged via slog.

## Testing

- **Unit:** token maker, password hash, currency validation, error mapping (no DB).
- **Integration** (`-tags=integration`): sqlc queries + `TransferTx` concurrency
  test against `postgres-test` (port 5433); mail against `mailpit-test`.
- Store/mail consumers use interfaces → handlers tested with mock `Store` / `Mailer`.

## Security Hardening

- bcrypt (cost ≥ 12); never log secrets or tokens.
- JWT secret min-length enforced; short access TTL; refresh rotation + session
  block/revoke.
- Existing middleware: `Secure`, `BodyLimit`, `RequestID`, `Recover`,
  `ContextTimeout`.
- Add: per-IP rate limit on auth endpoints (`golang.org/x/time/rate`); DTO input
  validation; parameterized queries (sqlc → no SQL injection); TLS terminated at
  ingress.
- `govulncheck` + `golangci-lint` in CI (already wired).

## Operations (container-only)

- slog JSON to stdout.
- Layered timeouts: Echo `ContextTimeout` (30s outer) with per-dependency
  `context.WithTimeout` (shorter inner deadlines).
- Graceful shutdown for both `serve` and `worker` via `signal.NotifyContext`.
- Probe split: `livez` (no deps) vs `readyz` (DB ping).
- Stateless instances — scale horizontally.

## Adoption Order

1. Config/CLI + pgxpool + goose migrations + sqlc setup (schema, generated code).
2. Store layer + `execTx` + `TransferTx` + concurrency tests.
3. Auth: token maker, users register/login, sessions, `echo-jwt` middleware.
4. Accounts + transfers HTTP endpoints + error mapping.
5. River integration: transactional enqueue + worker + go-mail + verify-email flow.
6. Security hardening (rate limit, validation) + ops polish (probes, timeouts).
```
