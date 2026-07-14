# Task 4: Domain schema migrations (goose)

**Files:**
- Create: `internal/db/migrations/00001_init_schema.sql`
- Create: `internal/db/migrations/embed.go`

## Produces
- Package `migrations` at `internal/db/migrations` exporting `var FS embed.FS` (embeds `*.sql`).
- Initial goose migration creating all domain tables.

## Step 1: Write `internal/db/migrations/00001_init_schema.sql`
```sql
-- +goose Up
CREATE TABLE users (
    id                  uuid PRIMARY KEY DEFAULT uuidv7(),
    username            text UNIQUE NOT NULL,
    hashed_password     text NOT NULL,
    full_name           text NOT NULL,
    email               text UNIQUE NOT NULL,
    is_email_verified   boolean NOT NULL DEFAULT false,
    password_changed_at timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z',
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE verify_emails (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    username    text NOT NULL REFERENCES users(username),
    email       text NOT NULL,
    secret_code text NOT NULL,
    is_used     boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expired_at  timestamptz NOT NULL DEFAULT (now() + interval '15 minutes')
);

CREATE TABLE accounts (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    owner      text NOT NULL REFERENCES users(username),
    balance    bigint NOT NULL,
    currency   text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner, currency)
);
CREATE INDEX idx_accounts_owner ON accounts (owner);

CREATE TABLE entries (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES accounts(id),
    amount     bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_entries_account_id ON entries (account_id);

CREATE TABLE transfers (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    from_account_id uuid NOT NULL REFERENCES accounts(id),
    to_account_id   uuid NOT NULL REFERENCES accounts(id),
    amount          bigint NOT NULL CHECK (amount > 0),
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_transfers_from_account_id ON transfers (from_account_id);
CREATE INDEX idx_transfers_to_account_id ON transfers (to_account_id);

CREATE TABLE sessions (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    username      text NOT NULL REFERENCES users(username),
    refresh_token text NOT NULL,
    user_agent    text NOT NULL,
    client_ip     text NOT NULL,
    is_blocked    boolean NOT NULL DEFAULT false,
    expires_at    timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE sessions;
DROP TABLE transfers;
DROP TABLE entries;
DROP TABLE accounts;
DROP TABLE verify_emails;
DROP TABLE users;
```

## Step 2: Write `internal/db/migrations/embed.go`
```go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

## Step 3: Verify it compiles
`go build ./internal/db/migrations/` → no error.

## Step 4: Commit
```bash
git add internal/db/migrations/
git commit -m "feat: add initial domain schema goose migration"
```

## Global Constraints
- All primary keys are `uuid PRIMARY KEY DEFAULT uuidv7()` (Postgres 18 native `uuidv7()`; no extension).
- Money is `bigint` minor units. `transfers.amount` has `CHECK (amount > 0)`.
- Do NOT run migrations here (no DB connection); only create files.
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-4-report.md`. Return only: status, commit hash(es), one-line build summary, concerns.
