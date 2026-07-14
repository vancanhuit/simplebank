# SimpleBank Cloud-Native Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-grade "simplebank" REST service in Go with users/accounts/entries/transfers, JWT auth, atomic money transfers, and async email verification.

**Architecture:** Single binary with three urfave/cli v3 subcommands (`serve`, `migrate`, `worker`). Echo v5 HTTP API delegates to a pgx/v5 + sqlc `Store` (with `execTx`/`TransferTx`), a JWT `token.Maker`, a River queue `worker.Client`, and a go-mail `Mailer`. goose owns the domain schema (embedded); River owns its own tables. All dependencies are interfaces, constructor-injected.

**Tech Stack:** Go 1.26, Echo v5 (`github.com/labstack/echo/v5`), echo-jwt v5 (`github.com/labstack/echo-jwt/v5`), golang-jwt v5, pgx/v5 + pgxpool, sqlc, goose, River (`github.com/riverqueue/river`), go-mail (`github.com/wneessen/go-mail`), urfave/cli v3 (`github.com/urfave/cli/v3`), bcrypt, `golang.org/x/time/rate`, PostgreSQL 18.

## Global Constraints

- Go module path: `github.com/vancanhuit/simplebank`. Go version `1.26.5`.
- Echo v5 handler signature is `func(c *echo.Context) error` (pointer, not interface).
- Retrieve JWT token in handlers via `echo.ContextGet[*jwt.Token](c, "user")`.
- All primary keys are `uuid PRIMARY KEY DEFAULT uuidv7()` (Postgres 18 native `uuidv7()`; no extension).
- Money is stored as `bigint` minor units (cents). Never use floats for money.
- JSON APIs are served under `/api/v1`. Health probes `/livez` and `/readyz` stay at root.
- bcrypt cost must be `>= 12`.
- Never log secrets, passwords, tokens, or secret codes.
- pgx errors must be translated to stable app errors at the store boundary; `pgx.Tx` must never leave `internal/db`.
- Transaction callbacks must be pure DB — no email/HTTP/side effects inside a transaction.
- Config is loaded via urfave/cli v3 flags, each with an env value-source fallback; validate at startup and fail fast.
- Existing tooling: `mise` tasks, `golangci-lint`, `govulncheck`, Docker Compose profiles `dev` (Postgres :5432) and `test` (Postgres :5433, Mailpit).
- Unit tests run with `go test -race -cover ./...`; integration tests use build tag `integration` and the `test` compose profile.
- Use conventional commit messages (cocogitto verifies commit-msg).

---

### Task 1: Add dependencies and sqlc/goose tooling config

**Files:**
- Modify: `go.mod`
- Create: `sqlc.yaml`
- Modify: `mise.toml`

**Interfaces:**
- Consumes: nothing.
- Produces: module dependencies available; `mise run sqlc:generate` task; sqlc config pointing at `internal/db/migrations` (schema) and `internal/db/query` (queries), output to `internal/db/sqlc` as package `db`.

- [ ] **Step 1: Add Go dependencies**

Run:
```bash
go get github.com/labstack/echo-jwt/v5@latest
go get github.com/golang-jwt/jwt/v5@latest
go get github.com/jackc/pgx/v5@latest
go get github.com/pressly/goose/v3@latest
go get github.com/riverqueue/river@latest
go get github.com/riverqueue/river/riverdriver/riverpgxv5@latest
go get github.com/riverqueue/river/rivermigrate@latest
go get github.com/wneessen/go-mail@latest
go get github.com/urfave/cli/v3@latest
go get golang.org/x/crypto/bcrypt@latest
go get golang.org/x/time/rate@latest
```
Expected: `go.mod` lists these modules; no build errors.

- [ ] **Step 2: Create `sqlc.yaml`**

```yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "internal/db/migrations"
    queries: "internal/db/query"
    gen:
      go:
        package: "db"
        out: "internal/db/sqlc"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_interface: true
        emit_empty_slices: true
        overrides:
          - db_type: "uuid"
            go_type: "github.com/google/uuid.UUID"
          - db_type: "timestamptz"
            go_type: "time.Time"
```

- [ ] **Step 3: Add sqlc install and generate tasks to `mise.toml`**

Add these tasks under the existing `[tasks...]` entries:
```toml
[tasks."sqlc:generate"]
description = "Generate Go code from SQL with sqlc"
run = [
  'go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest',
  'sqlc generate',
]
```

- [ ] **Step 4: Add google/uuid dependency (used by sqlc overrides)**

Run: `go get github.com/google/uuid@latest`
Expected: `go.mod` includes `github.com/google/uuid`.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum sqlc.yaml mise.toml
git commit -m "chore: add pgx sqlc goose river go-mail cli deps and sqlc config"
```

---

### Task 2: Config package (urfave/cli flags + env value sources)

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Config struct { HTTPAddr string; DBSource string; JWTSecret string; AccessTTL time.Duration; RefreshTTL time.Duration; SMTPHost string; SMTPPort int; SMTPUsername string; SMTPPassword string; SMTPFrom string; RiverMaxWorkers int }`
  - `func (c Config) Validate() error` — returns error if `DBSource == ""`, `JWTSecret` shorter than 32 chars, or `SMTPFrom == ""`.
  - `func Flags() []cli.Flag` — returns urfave/cli v3 flags each with `Sources: cli.EnvVars("...")`.
  - `func FromCommand(cmd *cli.Command) Config` — builds `Config` from parsed `*cli.Command`.

- [ ] **Step 1: Write the failing test**

```go
package config

import "testing"

func TestValidate(t *testing.T) {
	valid := Config{
		DBSource:  "postgres://u:p@localhost:5432/db",
		JWTSecret: "01234567890123456789012345678901",
		SMTPFrom:  "no-reply@example.com",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	tests := map[string]Config{
		"missing db":     {JWTSecret: "01234567890123456789012345678901", SMTPFrom: "a@b.c"},
		"short secret":   {DBSource: "x", JWTSecret: "short", SMTPFrom: "a@b.c"},
		"missing from":   {DBSource: "x", JWTSecret: "01234567890123456789012345678901"},
	}
	for name, cfg := range tests {
		if err := cfg.Validate(); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestValidate -v`
Expected: FAIL (package/type does not compile).

- [ ] **Step 3: Write the implementation**

```go
package config

import (
	"errors"
	"time"

	"github.com/urfave/cli/v3"
)

type Config struct {
	HTTPAddr        string
	DBSource        string
	JWTSecret       string
	AccessTTL       time.Duration
	RefreshTTL      time.Duration
	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string
	RiverMaxWorkers int
}

func (c Config) Validate() error {
	if c.DBSource == "" {
		return errors.New("db-source is required")
	}
	if len(c.JWTSecret) < 32 {
		return errors.New("jwt-secret must be at least 32 characters")
	}
	if c.SMTPFrom == "" {
		return errors.New("smtp-from is required")
	}
	return nil
}

func Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "http-addr", Value: ":8080", Sources: cli.EnvVars("HTTP_ADDR")},
		&cli.StringFlag{Name: "db-source", Sources: cli.EnvVars("DB_SOURCE")},
		&cli.StringFlag{Name: "jwt-secret", Sources: cli.EnvVars("JWT_SECRET")},
		&cli.DurationFlag{Name: "access-ttl", Value: 15 * time.Minute, Sources: cli.EnvVars("ACCESS_TTL")},
		&cli.DurationFlag{Name: "refresh-ttl", Value: 24 * time.Hour, Sources: cli.EnvVars("REFRESH_TTL")},
		&cli.StringFlag{Name: "smtp-host", Sources: cli.EnvVars("SMTP_HOST")},
		&cli.IntFlag{Name: "smtp-port", Value: 1025, Sources: cli.EnvVars("SMTP_PORT")},
		&cli.StringFlag{Name: "smtp-username", Sources: cli.EnvVars("SMTP_USERNAME")},
		&cli.StringFlag{Name: "smtp-password", Sources: cli.EnvVars("SMTP_PASSWORD")},
		&cli.StringFlag{Name: "smtp-from", Sources: cli.EnvVars("SMTP_FROM")},
		&cli.IntFlag{Name: "river-max-workers", Value: 10, Sources: cli.EnvVars("RIVER_MAX_WORKERS")},
	}
}

func FromCommand(cmd *cli.Command) Config {
	return Config{
		HTTPAddr:        cmd.String("http-addr"),
		DBSource:        cmd.String("db-source"),
		JWTSecret:       cmd.String("jwt-secret"),
		AccessTTL:       cmd.Duration("access-ttl"),
		RefreshTTL:      cmd.Duration("refresh-ttl"),
		SMTPHost:        cmd.String("smtp-host"),
		SMTPPort:        cmd.Int("smtp-port"),
		SMTPUsername:    cmd.String("smtp-username"),
		SMTPPassword:    cmd.String("smtp-password"),
		SMTPFrom:        cmd.String("smtp-from"),
		RiverMaxWorkers: cmd.Int("river-max-workers"),
	}
}
```

Note: if `cmd.Int` returns `int64` in the installed cli version, change `SMTPPort`/`RiverMaxWorkers` to `int64` or cast with `int(...)`; adjust the struct field types to match and keep the test compiling.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestValidate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config with cli flags and env value sources"
```

---

### Task 3: Utility helpers (password hashing, random, currency)

**Files:**
- Create: `internal/util/password.go`
- Create: `internal/util/random.go`
- Create: `internal/util/currency.go`
- Test: `internal/util/password_test.go`
- Test: `internal/util/currency_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `func HashPassword(password string) (string, error)`
  - `func CheckPassword(password, hashedPassword string) error` (nil if match)
  - `func RandomString(n int) string`
  - `func RandomOwner() string`
  - `func IsSupportedCurrency(currency string) bool` (USD, EUR, VND)
  - `const (USD = "USD"; EUR = "EUR"; VND = "VND")`

- [ ] **Step 1: Write the failing tests**

`internal/util/password_test.go`:
```go
package util

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPassword(t *testing.T) {
	password := RandomString(8)

	hashed, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := CheckPassword(password, hashed); err != nil {
		t.Fatalf("check should pass: %v", err)
	}
	if err := CheckPassword("wrong", hashed); err != bcrypt.ErrMismatchedHashAndPassword {
		t.Fatalf("check should fail with mismatch, got %v", err)
	}

	hashed2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash2: %v", err)
	}
	if hashed == hashed2 {
		t.Fatal("hashes should differ due to salt")
	}
}
```

`internal/util/currency_test.go`:
```go
package util

import "testing"

func TestIsSupportedCurrency(t *testing.T) {
	if !IsSupportedCurrency(USD) {
		t.Error("USD should be supported")
	}
	if IsSupportedCurrency("XYZ") {
		t.Error("XYZ should not be supported")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/util/ -v`
Expected: FAIL (undefined functions).

- [ ] **Step 3: Write the implementations**

`internal/util/password.go`:
```go
package util

import "golang.org/x/crypto/bcrypt"

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func CheckPassword(password, hashedPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
```

`internal/util/random.go`:
```go
package util

import (
	"math/rand"
	"strings"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz"

func RandomString(n int) string {
	var sb strings.Builder
	for range n {
		sb.WriteByte(alphabet[rand.Intn(len(alphabet))])
	}
	return sb.String()
}

func RandomOwner() string {
	return RandomString(6)
}
```

`internal/util/currency.go`:
```go
package util

const (
	USD = "USD"
	EUR = "EUR"
	VND = "VND"
)

func IsSupportedCurrency(currency string) bool {
	switch currency {
	case USD, EUR, VND:
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/util/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/util/
git commit -m "feat: add password hashing, random, and currency helpers"
```

---

### Task 4: Domain schema migrations (goose)

**Files:**
- Create: `internal/db/migrations/00001_init_schema.sql`
- Create: `internal/db/migrations/embed.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `db.MigrationsFS embed.FS` exported from package `db` in `internal/db/migrations` — NOTE: to avoid a package name clash with sqlc output, this embed lives in its own package `migrations`. Export `migrations.FS embed.FS`.

- [ ] **Step 1: Write the migration SQL**

`internal/db/migrations/00001_init_schema.sql`:
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

- [ ] **Step 2: Write the embed file**

`internal/db/migrations/embed.go`:
```go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/db/migrations/`
Expected: builds with no error.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/
git commit -m "feat: add initial domain schema goose migration"
```

---

### Task 5: sqlc queries and generated code

**Files:**
- Create: `internal/db/query/users.sql`
- Create: `internal/db/query/accounts.sql`
- Create: `internal/db/query/entries.sql`
- Create: `internal/db/query/transfers.sql`
- Create: `internal/db/query/sessions.sql`
- Create: `internal/db/query/verify_emails.sql`
- Generated (do not hand-edit): `internal/db/sqlc/*.go`

**Interfaces:**
- Consumes: schema from Task 4.
- Produces (package `db` in `internal/db/sqlc`): `Querier` interface and `Queries` struct with methods including `CreateUser`, `GetUser`, `CreateAccount`, `GetAccount`, `GetAccountForUpdate`, `ListAccounts`, `AddAccountBalance`, `CreateEntry`, `CreateTransfer`, `CreateSession`, `GetSession`, `CreateVerifyEmail`, `UpdateVerifyEmail`; model structs `User`, `Account`, `Entry`, `Transfer`, `Session`, `VerifyEmail`; and `type DBTX interface`. `New(db DBTX) *Queries` and `(*Queries).WithTx(tx pgx.Tx) *Queries`.

- [ ] **Step 1: Write `users.sql`**

```sql
-- name: CreateUser :one
INSERT INTO users (username, hashed_password, full_name, email)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE username = $1 LIMIT 1;

-- name: VerifyUserEmail :one
UPDATE users SET is_email_verified = true
WHERE username = $1
RETURNING *;
```

- [ ] **Step 2: Write `accounts.sql`**

```sql
-- name: CreateAccount :one
INSERT INTO accounts (owner, balance, currency)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = $1 LIMIT 1;

-- name: GetAccountForUpdate :one
SELECT * FROM accounts WHERE id = $1 LIMIT 1 FOR NO KEY UPDATE;

-- name: ListAccounts :many
SELECT * FROM accounts
WHERE owner = $1
ORDER BY id
LIMIT $2 OFFSET $3;

-- name: AddAccountBalance :one
UPDATE accounts
SET balance = balance + sqlc.arg(amount)
WHERE id = sqlc.arg(id) AND balance + sqlc.arg(amount) >= 0
RETURNING *;
```

- [ ] **Step 3: Write `entries.sql`, `transfers.sql`, `sessions.sql`, `verify_emails.sql`**

`entries.sql`:
```sql
-- name: CreateEntry :one
INSERT INTO entries (account_id, amount)
VALUES ($1, $2)
RETURNING *;
```

`transfers.sql`:
```sql
-- name: CreateTransfer :one
INSERT INTO transfers (from_account_id, to_account_id, amount)
VALUES ($1, $2, $3)
RETURNING *;
```

`sessions.sql`:
```sql
-- name: CreateSession :one
INSERT INTO sessions (id, username, refresh_token, user_agent, client_ip, is_blocked, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1 LIMIT 1;
```

`verify_emails.sql`:
```sql
-- name: CreateVerifyEmail :one
INSERT INTO verify_emails (username, email, secret_code)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateVerifyEmail :one
UPDATE verify_emails
SET is_used = true
WHERE id = sqlc.arg(id)
  AND secret_code = sqlc.arg(secret_code)
  AND is_used = false
  AND expired_at > now()
RETURNING *;
```

- [ ] **Step 4: Generate sqlc code**

Run: `mise run sqlc:generate`
Expected: files created under `internal/db/sqlc/` (`db.go`, `models.go`, `querier.go`, `*.sql.go`); `go build ./internal/db/sqlc/` passes.

- [ ] **Step 5: Commit**

```bash
git add internal/db/query/ internal/db/sqlc/
git commit -m "feat: add sqlc queries and generated db code"
```

---

### Task 6: Store interface, execTx, and error translation

**Files:**
- Create: `internal/db/store.go`
- Create: `internal/db/errors.go`
- Test: `internal/db/errors_test.go`

**Interfaces:**
- Consumes: generated `db` package (Task 5). NOTE: `internal/db/sqlc` is package `db`; import it as `sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"` inside `internal/db` (package `store`). Package for `internal/db/store.go` is `store`.
- Produces (package `store`):
  - `type Store interface { sqlcdb.Querier; TransferTx(ctx, TransferTxParams) (TransferTxResult, error) }`
  - `type SQLStore struct { *sqlcdb.Queries; connPool *pgxpool.Pool }`
  - `func NewStore(pool *pgxpool.Pool) Store`
  - `func (s *SQLStore) execTx(ctx context.Context, fn func(*sqlcdb.Queries) error) error`
  - Sentinel errors: `ErrRecordNotFound`, `ErrUniqueViolation`, `ErrForeignKeyViolation`, `ErrInsufficientBalance`.
  - `func ClassifyError(err error) error` — maps `pgx.ErrNoRows` → `ErrRecordNotFound`, `*pgconn.PgError` codes `23505` → `ErrUniqueViolation`, `23503` → `ErrForeignKeyViolation`; otherwise returns `err`.

- [ ] **Step 1: Write the failing test**

`internal/db/errors_test.go`:
```go
package store

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestClassifyError(t *testing.T) {
	if !errors.Is(ClassifyError(pgx.ErrNoRows), ErrRecordNotFound) {
		t.Error("ErrNoRows should map to ErrRecordNotFound")
	}
	uniq := &pgconn.PgError{Code: "23505"}
	if !errors.Is(ClassifyError(uniq), ErrUniqueViolation) {
		t.Error("23505 should map to ErrUniqueViolation")
	}
	fk := &pgconn.PgError{Code: "23503"}
	if !errors.Is(ClassifyError(fk), ErrForeignKeyViolation) {
		t.Error("23503 should map to ErrForeignKeyViolation")
	}
	other := errors.New("boom")
	if ClassifyError(other) != other {
		t.Error("unknown error should pass through unchanged")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestClassifyError -v`
Expected: FAIL (undefined symbols).

- [ ] **Step 3: Write `errors.go`**

```go
package store

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrRecordNotFound      = errors.New("record not found")
	ErrUniqueViolation     = errors.New("unique constraint violation")
	ErrForeignKeyViolation = errors.New("foreign key violation")
	ErrInsufficientBalance = errors.New("insufficient balance")
)

func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRecordNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrUniqueViolation
		case "23503":
			return ErrForeignKeyViolation
		}
	}
	return err
}
```

- [ ] **Step 4: Write `store.go`**

```go
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type Store interface {
	sqlcdb.Querier
	TransferTx(ctx context.Context, arg TransferTxParams) (TransferTxResult, error)
}

type SQLStore struct {
	*sqlcdb.Queries
	connPool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return &SQLStore{
		Queries:  sqlcdb.New(pool),
		connPool: pool,
	}
}

func (s *SQLStore) execTx(ctx context.Context, fn func(*sqlcdb.Queries) error) error {
	tx, err := s.connPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := sqlcdb.New(tx)
	if err := fn(q); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var _ = fmt.Sprintf
```

Note: remove the `var _ = fmt.Sprintf` line and the `fmt` import once `TransferTx` (Task 7) is added — it is a temporary compile aid only if no other use of `fmt` exists. Prefer to add Task 7 in the same session so this placeholder is unnecessary; if adding both together, omit the `fmt` import here.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestClassifyError -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/store.go internal/db/errors.go internal/db/errors_test.go
git commit -m "feat: add store interface, execTx, and pgx error translation"
```

---

### Task 7: TransferTx with deterministic lock order

**Files:**
- Create: `internal/db/transfer_tx.go`
- Modify: `internal/db/store.go` (remove the temporary `fmt` placeholder if present)

**Interfaces:**
- Consumes: `execTx`, `sqlcdb.Queries` methods `CreateTransfer`, `CreateEntry`, `AddAccountBalance`.
- Produces:
  - `type TransferTxParams struct { FromAccountID uuid.UUID; ToAccountID uuid.UUID; Amount int64 }`
  - `type TransferTxResult struct { Transfer sqlcdb.Transfer; FromAccount sqlcdb.Account; ToAccount sqlcdb.Account; FromEntry sqlcdb.Entry; ToEntry sqlcdb.Entry }`
  - `func (s *SQLStore) TransferTx(ctx, TransferTxParams) (TransferTxResult, error)`

- [ ] **Step 1: Write `transfer_tx.go`**

```go
package store

import (
	"context"
	"errors"

	"github.com/google/uuid"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type TransferTxParams struct {
	FromAccountID uuid.UUID `json:"from_account_id"`
	ToAccountID   uuid.UUID `json:"to_account_id"`
	Amount        int64     `json:"amount"`
}

type TransferTxResult struct {
	Transfer    sqlcdb.Transfer `json:"transfer"`
	FromAccount sqlcdb.Account  `json:"from_account"`
	ToAccount   sqlcdb.Account  `json:"to_account"`
	FromEntry   sqlcdb.Entry    `json:"from_entry"`
	ToEntry     sqlcdb.Entry    `json:"to_entry"`
}

func (s *SQLStore) TransferTx(ctx context.Context, arg TransferTxParams) (TransferTxResult, error) {
	var result TransferTxResult

	err := s.execTx(ctx, func(q *sqlcdb.Queries) error {
		var err error

		result.Transfer, err = q.CreateTransfer(ctx, sqlcdb.CreateTransferParams{
			FromAccountID: arg.FromAccountID,
			ToAccountID:   arg.ToAccountID,
			Amount:        arg.Amount,
		})
		if err != nil {
			return ClassifyError(err)
		}

		result.FromEntry, err = q.CreateEntry(ctx, sqlcdb.CreateEntryParams{
			AccountID: arg.FromAccountID,
			Amount:    -arg.Amount,
		})
		if err != nil {
			return ClassifyError(err)
		}
		result.ToEntry, err = q.CreateEntry(ctx, sqlcdb.CreateEntryParams{
			AccountID: arg.ToAccountID,
			Amount:    arg.Amount,
		})
		if err != nil {
			return ClassifyError(err)
		}

		// Deterministic lock order: update the smaller UUID first to avoid deadlocks.
		if arg.FromAccountID.String() < arg.ToAccountID.String() {
			result.FromAccount, result.ToAccount, err = moveMoney(ctx, q,
				arg.FromAccountID, -arg.Amount, arg.ToAccountID, arg.Amount)
		} else {
			result.ToAccount, result.FromAccount, err = moveMoney(ctx, q,
				arg.ToAccountID, arg.Amount, arg.FromAccountID, -arg.Amount)
		}
		return err
	})

	return result, err
}

func moveMoney(
	ctx context.Context,
	q *sqlcdb.Queries,
	id1 uuid.UUID, amount1 int64,
	id2 uuid.UUID, amount2 int64,
) (account1, account2 sqlcdb.Account, err error) {
	account1, err = q.AddAccountBalance(ctx, sqlcdb.AddAccountBalanceParams{
		ID:     id1,
		Amount: amount1,
	})
	if err != nil {
		return account1, account2, mapBalanceError(err)
	}
	account2, err = q.AddAccountBalance(ctx, sqlcdb.AddAccountBalanceParams{
		ID:     id2,
		Amount: amount2,
	})
	if err != nil {
		return account1, account2, mapBalanceError(err)
	}
	return account1, account2, nil
}

// mapBalanceError treats "no row updated" (pgx.ErrNoRows from AddAccountBalance's
// RETURNING when the balance guard fails) as insufficient balance.
func mapBalanceError(err error) error {
	classified := ClassifyError(err)
	if errors.Is(classified, ErrRecordNotFound) {
		return ErrInsufficientBalance
	}
	return classified
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/db/`
Expected: builds. If `internal/db/store.go` still has the `fmt` placeholder, remove that line and the `fmt` import now.

- [ ] **Step 3: Commit**

```bash
git add internal/db/transfer_tx.go internal/db/store.go
git commit -m "feat: add TransferTx with atomic balance guard and deterministic lock order"
```

---

### Task 8: Integration test for store (migrations + TransferTx concurrency)

**Files:**
- Create: `internal/db/main_test.go`
- Create: `internal/db/transfer_tx_test.go`

**Interfaces:**
- Consumes: `NewStore`, `TransferTx`, generated `db` queries, goose migrations FS.
- Produces: `testStore Store` package-global available to integration tests, established in `TestMain` under build tag `integration`.

- [ ] **Step 1: Write `main_test.go`**

```go
//go:build integration

package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/vancanhuit/simplebank/internal/db/migrations"
)

var testStore Store

func TestMain(m *testing.M) {
	ctx := context.Background()
	dsn := os.Getenv("DB_SOURCE")
	if dsn == "" {
		dsn = "postgres://simplebank_test:simplebank_test@localhost:5433/simplebank_test?sslmode=disable"
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}

	// Run goose migrations using a database/sql handle backed by pgx stdlib.
	sqlDB := stdlib.OpenDBFromPool(pool)
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		panic(err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		panic(err)
	}
	_ = sql.LevelDefault // keep database/sql import used if needed

	testStore = NewStore(pool)
	os.Exit(m.Run())
}
```

Note: verify `stdlib.OpenDBFromPool` exists in the installed pgx version; if not, open a separate `sql.Open("pgx", dsn)` handle for goose. Remove the `_ = sql.LevelDefault` line and the `database/sql` import if unused.

- [ ] **Step 2: Write `transfer_tx_test.go`**

```go
//go:build integration

package store

import (
	"context"
	"testing"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/util"
)

func createTestUser(t *testing.T) sqlcdb.User {
	t.Helper()
	hashed, err := util.HashPassword(util.RandomString(8))
	if err != nil {
		t.Fatal(err)
	}
	user, err := testStore.CreateUser(context.Background(), sqlcdb.CreateUserParams{
		Username:       util.RandomOwner(),
		HashedPassword: hashed,
		FullName:       util.RandomOwner(),
		Email:          util.RandomString(6) + "@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func createTestAccount(t *testing.T, owner string) sqlcdb.Account {
	t.Helper()
	acc, err := testStore.CreateAccount(context.Background(), sqlcdb.CreateAccountParams{
		Owner:    owner,
		Balance:  1000,
		Currency: util.USD,
	})
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

func TestTransferTxConcurrent(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	n := 10
	amount := int64(10)
	errs := make(chan error, n)

	for range n {
		go func() {
			_, err := testStore.TransferTx(context.Background(), TransferTxParams{
				FromAccountID: acc1.ID,
				ToAccountID:   acc2.ID,
				Amount:        amount,
			})
			errs <- err
		}()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Fatalf("transfer failed: %v", err)
		}
	}

	updated1, _ := testStore.GetAccount(context.Background(), acc1.ID)
	updated2, _ := testStore.GetAccount(context.Background(), acc2.ID)
	if updated1.Balance != 1000-int64(n)*amount {
		t.Errorf("acc1 balance = %d, want %d", updated1.Balance, 1000-int64(n)*amount)
	}
	if updated2.Balance != 1000+int64(n)*amount {
		t.Errorf("acc2 balance = %d, want %d", updated2.Balance, 1000+int64(n)*amount)
	}
}

func TestTransferTxInsufficientBalance(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	_, err := testStore.TransferTx(context.Background(), TransferTxParams{
		FromAccountID: acc1.ID,
		ToAccountID:   acc2.ID,
		Amount:        100000,
	})
	if err != ErrInsufficientBalance {
		t.Fatalf("want ErrInsufficientBalance, got %v", err)
	}
}
```

- [ ] **Step 3: Run integration tests**

Run: `mise run test:integration`
Expected: compose `test` profile starts, migrations apply, both tests PASS, compose torn down.

- [ ] **Step 4: Commit**

```bash
git add internal/db/main_test.go internal/db/transfer_tx_test.go
git commit -m "test: add store integration and TransferTx concurrency tests"
```

---

### Task 9: JWT token maker

**Files:**
- Create: `internal/token/maker.go`
- Create: `internal/token/jwt_maker.go`
- Test: `internal/token/jwt_maker_test.go`

**Interfaces:**
- Consumes: `github.com/golang-jwt/jwt/v5`, `github.com/google/uuid`.
- Produces:
  - `type Payload struct { ID uuid.UUID; Username string; Role string; jwt.RegisteredClaims }`
  - `func NewPayload(username, role string, duration time.Duration) (*Payload, error)`
  - `var ErrExpiredToken; var ErrInvalidToken`
  - `type Maker interface { CreateToken(username, role string, duration time.Duration) (string, *Payload, error); VerifyToken(token string) (*Payload, error) }`
  - `type JWTMaker struct { secretKey string }`
  - `func NewJWTMaker(secretKey string) (*JWTMaker, error)` (errors if `len(secretKey) < 32`)

- [ ] **Step 1: Write the failing test**

```go
package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTMaker(t *testing.T) {
	maker, err := NewJWTMaker("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	token, payload, err := maker.CreateToken("alice", "depositor", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || payload == nil {
		t.Fatal("expected token and payload")
	}
	got, err := maker.VerifyToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "alice" || got.Role != "depositor" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestJWTMakerExpired(t *testing.T) {
	maker, _ := NewJWTMaker("01234567890123456789012345678901")
	token, _, _ := maker.CreateToken("alice", "depositor", -time.Minute)
	_, err := maker.VerifyToken(token)
	if err != ErrExpiredToken {
		t.Fatalf("want ErrExpiredToken, got %v", err)
	}
}

func TestJWTMakerInvalidAlg(t *testing.T) {
	payload, _ := NewPayload("alice", "depositor", time.Minute)
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodNone, payload)
	signed, _ := jwtToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
	maker, _ := NewJWTMaker("01234567890123456789012345678901")
	if _, err := maker.VerifyToken(signed); err != ErrInvalidToken {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/token/ -v`
Expected: FAIL (undefined types).

- [ ] **Step 3: Write `maker.go`**

```go
package token

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrExpiredToken = errors.New("token has expired")
	ErrInvalidToken = errors.New("token is invalid")
)

type Payload struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
	jwt.RegisteredClaims
}

func NewPayload(username, role string, duration time.Duration) (*Payload, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return &Payload{
		ID:       id,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        id.String(),
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
		},
	}, nil
}

type Maker interface {
	CreateToken(username, role string, duration time.Duration) (string, *Payload, error)
	VerifyToken(token string) (*Payload, error)
}
```

- [ ] **Step 4: Write `jwt_maker.go`**

```go
package token

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTMaker struct {
	secretKey string
}

func NewJWTMaker(secretKey string) (*JWTMaker, error) {
	if len(secretKey) < 32 {
		return nil, errors.New("secret key must be at least 32 characters")
	}
	return &JWTMaker{secretKey: secretKey}, nil
}

func (m *JWTMaker) CreateToken(username, role string, duration time.Duration) (string, *Payload, error) {
	payload, err := NewPayload(username, role, duration)
	if err != nil {
		return "", nil, err
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)
	signed, err := jwtToken.SignedString([]byte(m.secretKey))
	return signed, payload, err
}

func (m *JWTMaker) VerifyToken(token string) (*Payload, error) {
	keyFunc := func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(m.secretKey), nil
	}
	parsed, err := jwt.ParseWithClaims(token, &Payload{}, keyFunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	payload, ok := parsed.Claims.(*Payload)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	return payload, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/token/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/token/
git commit -m "feat: add JWT token maker with access and refresh payloads"
```

---

### Task 10: Mailer interface and go-mail SMTP implementation

**Files:**
- Create: `internal/mail/mailer.go`
- Create: `internal/mail/smtp.go`

**Interfaces:**
- Consumes: `github.com/wneessen/go-mail`, `internal/config`.
- Produces:
  - `type Mailer interface { Send(ctx context.Context, to, subject, htmlBody string) error }`
  - `type SMTPMailer struct { ... }`
  - `func NewSMTPMailer(cfg config.Config) (*SMTPMailer, error)`

- [ ] **Step 1: Write `mailer.go`**

```go
package mail

import "context"

type Mailer interface {
	Send(ctx context.Context, to, subject, htmlBody string) error
}
```

- [ ] **Step 2: Write `smtp.go`**

```go
package mail

import (
	"context"

	"github.com/wneessen/go-mail"

	"github.com/vancanhuit/simplebank/internal/config"
)

type SMTPMailer struct {
	client *mail.Client
	from   string
}

func NewSMTPMailer(cfg config.Config) (*SMTPMailer, error) {
	opts := []mail.Option{
		mail.WithPort(cfg.SMTPPort),
		mail.WithTLSPolicy(mail.NoTLS),
	}
	if cfg.SMTPUsername != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(cfg.SMTPUsername),
			mail.WithPassword(cfg.SMTPPassword),
			mail.WithTLSPolicy(mail.TLSMandatory),
		)
	}
	client, err := mail.NewClient(cfg.SMTPHost, opts...)
	if err != nil {
		return nil, err
	}
	return &SMTPMailer{client: client, from: cfg.SMTPFrom}, nil
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, htmlBody string) error {
	msg := mail.NewMsg()
	if err := msg.From(m.from); err != nil {
		return err
	}
	if err := msg.To(to); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextHTML, htmlBody)
	return m.client.DialAndSendWithContext(ctx, msg)
}
```

Note: confirm go-mail option/method names (`WithTLSPolicy`, `NoTLS`, `TLSMandatory`, `SMTPAuthPlain`, `DialAndSendWithContext`, `SetBodyString`, `TypeTextHTML`) against the installed version; adjust to the exact exported names if they differ. Keep the `Mailer` interface stable regardless.

- [ ] **Step 3: Verify build**

Run: `go build ./internal/mail/`
Expected: builds.

- [ ] **Step 4: Commit**

```bash
git add internal/mail/
git commit -m "feat: add generic SMTP mailer using go-mail"
```

---

### Task 11: River job args, worker, and client

**Files:**
- Create: `internal/worker/verify_email.go`
- Create: `internal/worker/client.go`

**Interfaces:**
- Consumes: River, `internal/db` store, `internal/mail`, `internal/util`, generated `db`.
- Produces:
  - `type SendVerifyEmailArgs struct { Username string }` with `func (SendVerifyEmailArgs) Kind() string { return "send_verify_email" }`
  - `type SendVerifyEmailWorker struct { river.WorkerDefaults[SendVerifyEmailArgs]; store store.Store; mailer mail.Mailer; baseURL string }`
  - `func NewClient(ctx, pool *pgxpool.Pool, maxWorkers int, st store.Store, mailer mail.Mailer, baseURL string) (*river.Client[pgx.Tx], error)`
  - `func Migrate(ctx, pool *pgxpool.Pool) error` — runs River migrations.

- [ ] **Step 1: Write `verify_email.go`**

```go
package worker

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/mail"
	"github.com/vancanhuit/simplebank/internal/util"
)

type SendVerifyEmailArgs struct {
	Username string `json:"username"`
}

func (SendVerifyEmailArgs) Kind() string { return "send_verify_email" }

type SendVerifyEmailWorker struct {
	river.WorkerDefaults[SendVerifyEmailArgs]
	store   store.Store
	mailer  mail.Mailer
	baseURL string
}

func NewSendVerifyEmailWorker(st store.Store, mailer mail.Mailer, baseURL string) *SendVerifyEmailWorker {
	return &SendVerifyEmailWorker{store: st, mailer: mailer, baseURL: baseURL}
}

func (w *SendVerifyEmailWorker) Work(ctx context.Context, job *river.Job[SendVerifyEmailArgs]) error {
	user, err := w.store.GetUser(ctx, job.Args.Username)
	if err != nil {
		return err
	}

	ve, err := w.store.CreateVerifyEmail(ctx, sqlcdb.CreateVerifyEmailParams{
		Username:   user.Username,
		Email:      user.Email,
		SecretCode: util.RandomString(32),
	})
	if err != nil {
		return err
	}

	link := fmt.Sprintf("%s/api/v1/users/verify_email?id=%s&code=%s",
		w.baseURL, ve.ID.String(), ve.SecretCode)
	body := fmt.Sprintf(
		`Hello %s,<br/>Please <a href="%s">click here</a> to verify your email address.`,
		user.FullName, link)

	return w.mailer.Send(ctx, user.Email, "Welcome to SimpleBank", body)
}
```

- [ ] **Step 2: Write `client.go`**

```go
package worker

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/mail"
)

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

func NewClient(
	ctx context.Context,
	pool *pgxpool.Pool,
	maxWorkers int,
	st store.Store,
	mailer mail.Mailer,
	baseURL string,
) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, NewSendVerifyEmailWorker(st, mailer, baseURL))

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: maxWorkers},
		},
		Workers: workers,
	})
}
```

Note: verify River v0.x API names (`river.NewClient`, `river.Config`, `QueueConfig`, `river.AddWorker`, `rivermigrate.New`, `Migrate` signature) against the installed version and adjust if the API differs.

- [ ] **Step 3: Verify build**

Run: `go build ./internal/worker/`
Expected: builds.

- [ ] **Step 4: Commit**

```bash
git add internal/worker/
git commit -m "feat: add River verify-email worker and client"
```

---

### Task 12: API server scaffold, DTO validation, and error handler

**Files:**
- Create: `internal/api/server.go`
- Create: `internal/api/errors.go`
- Create: `internal/api/validator.go`
- Test: `internal/api/errors_test.go`

**Interfaces:**
- Consumes: Echo v5, `internal/db` store, `internal/token`, River client, `internal/config`.
- Produces:
  - `type Server struct { config config.Config; store store.Store; tokenMaker token.Maker; riverClient *river.Client[pgx.Tx]; router *echo.Echo }`
  - `func NewServer(cfg, store, maker, riverClient) (*Server, error)`
  - `func (s *Server) Handler() *echo.Echo`
  - `func errorHandler(c *echo.Context, err error)` mapping app errors to status codes.
  - `func toHTTPStatus(err error) int`

- [ ] **Step 1: Write the failing test**

`internal/api/errors_test.go`:
```go
package api

import (
	"net/http"
	"testing"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

func TestToHTTPStatus(t *testing.T) {
	cases := map[error]int{
		store.ErrRecordNotFound:      http.StatusNotFound,
		store.ErrUniqueViolation:     http.StatusConflict,
		store.ErrForeignKeyViolation: http.StatusConflict,
		store.ErrInsufficientBalance: http.StatusUnprocessableEntity,
		token.ErrExpiredToken:        http.StatusUnauthorized,
		token.ErrInvalidToken:        http.StatusUnauthorized,
	}
	for err, want := range cases {
		if got := toHTTPStatus(err); got != want {
			t.Errorf("%v: got %d want %d", err, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestToHTTPStatus -v`
Expected: FAIL.

- [ ] **Step 3: Write `errors.go`**

```go
package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

func toHTTPStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrRecordNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrUniqueViolation), errors.Is(err, store.ErrForeignKeyViolation):
		return http.StatusConflict
	case errors.Is(err, store.ErrInsufficientBalance):
		return http.StatusUnprocessableEntity
	case errors.Is(err, token.ErrExpiredToken), errors.Is(err, token.ErrInvalidToken):
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func errorHandler(c *echo.Context, err error) {
	if c.Response().Committed {
		return
	}

	var he *echo.HTTPError
	if errors.As(err, &he) {
		_ = c.JSON(he.StatusCode(), map[string]string{"error": he.Message})
		return
	}

	status := toHTTPStatus(err)
	message := "internal server error"
	if status != http.StatusInternalServerError {
		message = err.Error()
	} else {
		c.Logger().Error("request failed", "error", err)
	}
	_ = c.JSON(status, map[string]string{"error": message})
}
```

Note: confirm `echo.HTTPError` exposes `StatusCode()` and `Message` (it does in v5.3.0). `c.Logger()` returns `*slog.Logger`.

- [ ] **Step 4: Write `validator.go`**

```go
package api

import (
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
)

type customValidator struct {
	validate *validator.Validate
}

func (cv *customValidator) Validate(i any) error {
	if err := cv.validate.Struct(i); err != nil {
		return echo.NewHTTPError(400, err.Error())
	}
	return nil
}

func newValidator() *customValidator {
	return &customValidator{validate: validator.New()}
}
```

Run: `go get github.com/go-playground/validator/v10@latest` before building.

- [ ] **Step 5: Write `server.go`**

```go
package api

import (
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/riverqueue/river"

	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

type Server struct {
	config      config.Config
	store       store.Store
	tokenMaker  token.Maker
	riverClient *river.Client[pgx.Tx]
	router      *echo.Echo
}

func NewServer(
	cfg config.Config,
	st store.Store,
	maker token.Maker,
	riverClient *river.Client[pgx.Tx],
) (*Server, error) {
	e := echo.NewWithConfig(echo.Config{
		HTTPErrorHandler: errorHandler,
		Validator:        newValidator(),
	})

	e.Use(middleware.RequestID())
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Secure())
	e.Use(middleware.BodyLimit(1 << 20))
	e.Use(middleware.ContextTimeout(30 * time.Second))
	e.Use(middleware.Recover())

	s := &Server{
		config:      cfg,
		store:       st,
		tokenMaker:  maker,
		riverClient: riverClient,
		router:      e,
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) Handler() *echo.Echo { return s.router }
```

The `registerRoutes` method is added in Task 13. For this task, add a temporary stub in `server.go`:
```go
func (s *Server) registerRoutes() {
	s.router.GET("/livez", s.livez)
	s.router.GET("/readyz", s.readyz)
}
```
And add health handlers in `server.go`:
```go
import "net/http"

func (s *Server) livez(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(c *echo.Context) error {
	// Readiness depends on the database being reachable.
	if sqlStore, ok := s.store.(interface{ Ping(ctx any) error }); ok {
		_ = sqlStore
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
}
```
Note: a real DB ping is wired in Task 14 where the pool is available; for now `readyz` returns ready. Consolidate imports (`net/http`, `time`) into one import block.

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestToHTTPStatus -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/ go.mod go.sum
git commit -m "feat: add api server scaffold, error handler, and validator"
```

---

### Task 13: User, auth, account, and transfer handlers + routes + auth middleware

**Files:**
- Create: `internal/api/middleware.go`
- Create: `internal/api/user.go`
- Create: `internal/api/account.go`
- Create: `internal/api/transfer.go`
- Create: `internal/api/routes.go`
- Modify: `internal/api/server.go` (replace the temporary `registerRoutes` stub)

**Interfaces:**
- Consumes: `Server`, store, token maker, River client, echo-jwt.
- Produces:
  - `func (s *Server) authMiddleware() echo.MiddlewareFunc` using `echojwt.WithConfig`.
  - `func authPayload(c *echo.Context) (*token.Payload, error)` reading `echo.ContextGet[*jwt.Token](c, "user")` and returning the mapped `*token.Payload`.
  - Handlers: `createUser`, `loginUser`, `renewToken`, `verifyEmail`, `createAccount`, `getAccount`, `listAccounts`, `createTransfer`.
  - `func (s *Server) registerRoutes()` registering all `/api/v1` routes.

- [ ] **Step 1: Write `middleware.go`**

```go
package api

import (
	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v5"
	"github.com/labstack/echo/v5"

	"github.com/vancanhuit/simplebank/internal/token"
)

const authContextKey = "user"

func (s *Server) authMiddleware() echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		SigningKey: []byte(s.config.JWTSecret),
		ContextKey: authContextKey,
		NewClaimsFunc: func(c *echo.Context) jwt.Claims {
			return new(token.Payload)
		},
	})
}

func authPayload(c *echo.Context) (*token.Payload, error) {
	jwtToken, err := echo.ContextGet[*jwt.Token](c, authContextKey)
	if err != nil {
		return nil, echo.ErrUnauthorized
	}
	payload, ok := jwtToken.Claims.(*token.Payload)
	if !ok {
		return nil, echo.ErrUnauthorized
	}
	return payload, nil
}
```

Note: because `NewClaimsFunc` returns `*token.Payload`, the token in context parses claims into `*token.Payload` directly. Confirm echo-jwt populates `token.Claims` with this type.

- [ ] **Step 2: Write `user.go`**

```go
package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/util"
	"github.com/vancanhuit/simplebank/internal/worker"
)

type createUserRequest struct {
	Username string `json:"username" validate:"required,alphanum"`
	Password string `json:"password" validate:"required,min=6"`
	FullName string `json:"full_name" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
}

type userResponse struct {
	Username        string    `json:"username"`
	FullName        string    `json:"full_name"`
	Email           string    `json:"email"`
	IsEmailVerified bool      `json:"is_email_verified"`
	CreatedAt       time.Time `json:"created_at"`
}

func newUserResponse(u sqlcdb.User) userResponse {
	return userResponse{
		Username:        u.Username,
		FullName:        u.FullName,
		Email:           u.Email,
		IsEmailVerified: u.IsEmailVerified,
		CreatedAt:       u.CreatedAt,
	}
}

func (s *Server) createUser(c *echo.Context) error {
	var req createUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	hashed, err := util.HashPassword(req.Password)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	user, err := s.store.CreateUser(ctx, sqlcdb.CreateUserParams{
		Username:       req.Username,
		HashedPassword: hashed,
		FullName:       req.FullName,
		Email:          req.Email,
	})
	if err != nil {
		return store.ClassifyError(err)
	}

	if _, err := s.riverClient.Insert(ctx, worker.SendVerifyEmailArgs{Username: user.Username}, nil); err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, newUserResponse(user))
}
```

Note: `store.ClassifyError` is already applied inside store methods for TransferTx, but `CreateUser` is a direct sqlc call, so classify here. Confirm River client `Insert` signature `(ctx, args, *InsertOpts)`.

- [ ] **Step 3: Add login and token renewal to `user.go`**

Append:
```go
type loginUserRequest struct {
	Username string `json:"username" validate:"required,alphanum"`
	Password string `json:"password" validate:"required"`
}

type loginUserResponse struct {
	AccessToken           string       `json:"access_token"`
	AccessTokenExpiresAt  time.Time    `json:"access_token_expires_at"`
	RefreshToken          string       `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time    `json:"refresh_token_expires_at"`
	SessionID             string       `json:"session_id"`
	User                  userResponse `json:"user"`
}

func (s *Server) loginUser(c *echo.Context) error {
	var req loginUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	ctx := c.Request().Context()
	user, err := s.store.GetUser(ctx, req.Username)
	if err != nil {
		if err := store.ClassifyError(err); err == store.ErrRecordNotFound {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
		}
		return err
	}
	if err := util.CheckPassword(req.Password, user.HashedPassword); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}

	accessToken, accessPayload, err := s.tokenMaker.CreateToken(user.Username, "depositor", s.config.AccessTTL)
	if err != nil {
		return err
	}
	refreshToken, refreshPayload, err := s.tokenMaker.CreateToken(user.Username, "depositor", s.config.RefreshTTL)
	if err != nil {
		return err
	}

	session, err := s.store.CreateSession(ctx, sqlcdb.CreateSessionParams{
		ID:           refreshPayload.ID,
		Username:     user.Username,
		RefreshToken: refreshToken,
		UserAgent:    c.Request().UserAgent(),
		ClientIp:     c.RealIP(),
		IsBlocked:    false,
		ExpiresAt:    refreshPayload.ExpiresAt.Time,
	})
	if err != nil {
		return store.ClassifyError(err)
	}

	return c.JSON(http.StatusOK, loginUserResponse{
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessPayload.ExpiresAt.Time,
		RefreshToken:          refreshToken,
		RefreshTokenExpiresAt: refreshPayload.ExpiresAt.Time,
		SessionID:             session.ID.String(),
		User:                  newUserResponse(user),
	})
}

type renewTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func (s *Server) renewToken(c *echo.Context) error {
	var req renewTokenRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	refreshPayload, err := s.tokenMaker.VerifyToken(req.RefreshToken)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	session, err := s.store.GetSession(ctx, refreshPayload.ID)
	if err != nil {
		return store.ClassifyError(err)
	}
	if session.IsBlocked ||
		session.Username != refreshPayload.Username ||
		session.RefreshToken != req.RefreshToken ||
		time.Now().After(session.ExpiresAt) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
	}

	accessToken, accessPayload, err := s.tokenMaker.CreateToken(refreshPayload.Username, refreshPayload.Role, s.config.AccessTTL)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"access_token":            accessToken,
		"access_token_expires_at": accessPayload.ExpiresAt.Time,
	})
}
```

Note: confirm generated field names (`ClientIp` vs `ClientIP`) from sqlc output and match exactly.

- [ ] **Step 4: Add verify-email handler to `user.go`**

Append:
```go
func (s *Server) verifyEmail(c *echo.Context) error {
	idStr := c.QueryParam("id")
	code := c.QueryParam("code")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	ctx := c.Request().Context()
	ve, err := s.store.UpdateVerifyEmail(ctx, sqlcdb.UpdateVerifyEmailParams{
		ID:         id,
		SecretCode: code,
	})
	if err != nil {
		if err := store.ClassifyError(err); err == store.ErrRecordNotFound {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired verification link")
		}
		return err
	}
	if _, err := s.store.VerifyUserEmail(ctx, ve.Username); err != nil {
		return store.ClassifyError(err)
	}
	return c.JSON(http.StatusOK, map[string]bool{"is_verified": true})
}
```
Add `"github.com/google/uuid"` to imports.

- [ ] **Step 5: Write `account.go`**

```go
package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/util"
)

type createAccountRequest struct {
	Currency string `json:"currency" validate:"required"`
}

func (s *Server) createAccount(c *echo.Context) error {
	var req createAccountRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}
	if !util.IsSupportedCurrency(req.Currency) {
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported currency")
	}

	payload, err := authPayload(c)
	if err != nil {
		return err
	}

	account, err := s.store.CreateAccount(c.Request().Context(), sqlcdb.CreateAccountParams{
		Owner:    payload.Username,
		Balance:  0,
		Currency: req.Currency,
	})
	if err != nil {
		return store.ClassifyError(err)
	}
	return c.JSON(http.StatusCreated, account)
}

func (s *Server) getAccount(c *echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid account id")
	}

	account, err := s.store.GetAccount(c.Request().Context(), id)
	if err != nil {
		return store.ClassifyError(err)
	}

	payload, err := authPayload(c)
	if err != nil {
		return err
	}
	if account.Owner != payload.Username {
		return echo.NewHTTPError(http.StatusForbidden, "account does not belong to you")
	}
	return c.JSON(http.StatusOK, account)
}

func (s *Server) listAccounts(c *echo.Context) error {
	page, err := echo.QueryParamOr[int32](c, "page", 1)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid page")
	}
	size, err := echo.QueryParamOr[int32](c, "size", 5)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid size")
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 5
	}

	payload, err := authPayload(c)
	if err != nil {
		return err
	}

	accounts, err := s.store.ListAccounts(c.Request().Context(), sqlcdb.ListAccountsParams{
		Owner:  payload.Username,
		Limit:  size,
		Offset: (page - 1) * size,
	})
	if err != nil {
		return store.ClassifyError(err)
	}
	return c.JSON(http.StatusOK, accounts)
}
```

Note: confirm sqlc `ListAccountsParams` field types (`Limit`, `Offset` are usually `int32`). Adjust the generic param types to match.

- [ ] **Step 6: Write `transfer.go`**

```go
package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type transferRequest struct {
	FromAccountID string `json:"from_account_id" validate:"required,uuid"`
	ToAccountID   string `json:"to_account_id" validate:"required,uuid"`
	Amount        int64  `json:"amount" validate:"required,gt=0"`
	Currency      string `json:"currency" validate:"required"`
}

func (s *Server) createTransfer(c *echo.Context) error {
	var req transferRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	fromID, _ := uuid.Parse(req.FromAccountID)
	toID, _ := uuid.Parse(req.ToAccountID)
	ctx := c.Request().Context()

	fromAccount, err := s.validAccount(ctx, fromID, req.Currency)
	if err != nil {
		return err
	}
	if _, err := s.validAccount(ctx, toID, req.Currency); err != nil {
		return err
	}

	payload, err := authPayload(c)
	if err != nil {
		return err
	}
	if fromAccount.Owner != payload.Username {
		return echo.NewHTTPError(http.StatusForbidden, "from account does not belong to you")
	}

	result, err := s.store.TransferTx(ctx, store.TransferTxParams{
		FromAccountID: fromID,
		ToAccountID:   toID,
		Amount:        req.Amount,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

func (s *Server) validAccount(ctx context.Context, id uuid.UUID, currency string) (sqlcdb.Account, error) {
	account, err := s.store.GetAccount(ctx, id)
	if err != nil {
		return account, store.ClassifyError(err)
	}
	if account.Currency != currency {
		return account, echo.NewHTTPError(http.StatusBadRequest, "currency mismatch")
	}
	return account, nil
}
```
Add `"context"` to the import block.

- [ ] **Step 7: Write `routes.go` and replace the stub in `server.go`**

`routes.go`:
```go
package api

import "net/http"

import "github.com/labstack/echo/v5"

func (s *Server) registerRoutes() {
	s.router.GET("/livez", s.livez)
	s.router.GET("/readyz", s.readyz)

	v1 := s.router.Group("/api/v1")

	v1.POST("/users", s.createUser)
	v1.POST("/users/login", s.loginUser)
	v1.POST("/tokens/renew", s.renewToken)
	v1.GET("/users/verify_email", s.verifyEmail)

	auth := v1.Group("")
	auth.Use(s.authMiddleware())
	auth.POST("/accounts", s.createAccount)
	auth.GET("/accounts/:id", s.getAccount)
	auth.GET("/accounts", s.listAccounts)
	auth.POST("/transfers", s.createTransfer)
}

var _ = http.StatusOK
var _ echo.MiddlewareFunc
```

Remove the temporary `registerRoutes` stub from `server.go` (keep `livez`/`readyz` handlers there). Clean up the `var _ =` compile aids once everything builds.

- [ ] **Step 8: Verify build**

Run: `go build ./internal/api/`
Expected: builds. Fix any sqlc field-name mismatches surfaced by the compiler.

- [ ] **Step 9: Commit**

```bash
git add internal/api/
git commit -m "feat: add user, account, transfer handlers with JWT auth routes"
```

---

### Task 14: Migration runner + wire subcommands in main.go (serve, worker)

**Files:**
- Create: `internal/db/migrate.go`
- Modify: `cmd/app/main.go`

**Interfaces:**
- Consumes: everything above; goose Provider API + Postgres session locker.
- Produces:
  - `func MigrateSchema(ctx context.Context, pool *pgxpool.Pool) error` (package `store`) — runs domain goose migrations under a PostgreSQL session-level advisory lock.
  - A working CLI with two subcommands: `simplebank serve`, `simplebank worker`. Both run domain + River migrations on startup before proceeding. There is NO `migrate` subcommand.

- [ ] **Step 1: Create `internal/db/migrate.go` (goose Provider + session locker)**

```go
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/vancanhuit/simplebank/internal/db/migrations"
)

// MigrateSchema applies pending domain migrations. It uses a PostgreSQL
// session-level advisory lock so that when multiple replicas start together,
// only one applies migrations while the others wait, then all proceed.
func MigrateSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlDB := stdlib.OpenDBFromPool(pool)

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		sqlDB,
		migrations.FS,
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return err
	}

	_, err = provider.Up(ctx)
	return err
}
```

Note: verify the goose v3 Provider API against the installed version — `goose.NewProvider(dialect, *sql.DB, fs.FS, ...ProviderOption)`, `goose.DialectPostgres`, `goose.WithSessionLocker(locker)`, `lock.NewPostgresSessionLocker()` (package `github.com/pressly/goose/v3/lock`), and `provider.Up(ctx)`. Adjust names if the installed goose minor version differs. River migrations continue to use `worker.Migrate` (River applies its own advisory locking internally).

- [ ] **Step 2: Rewrite `cmd/app/main.go`**

```go
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/urfave/cli/v3"

	"github.com/vancanhuit/simplebank/internal/api"
	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/mail"
	"github.com/vancanhuit/simplebank/internal/token"
	"github.com/vancanhuit/simplebank/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cmd := &cli.Command{
		Name:  "simplebank",
		Usage: "SimpleBank cloud-native service",
		Flags: config.Flags(),
		Commands: []*cli.Command{
			{Name: "serve", Usage: "Run the HTTP API server", Action: runServe},
			{Name: "worker", Usage: "Run the background worker", Action: runWorker},
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.Run(ctx, os.Args); err != nil {
		logger.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func mustConfig(cmd *cli.Command) (config.Config, error) {
	cfg := config.FromCommand(cmd)
	return cfg, cfg.Validate()
}

// runMigrations applies domain (goose, session-locked) and River migrations.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if err := store.MigrateSchema(ctx, pool); err != nil {
		return err
	}
	slog.Info("domain migrations applied")
	if err := worker.Migrate(ctx, pool); err != nil {
		return err
	}
	slog.Info("river migrations applied")
	return nil
}

func runServe(ctx context.Context, cmd *cli.Command) error {
	cfg, err := mustConfig(cmd)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DBSource)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := runMigrations(ctx, pool); err != nil {
		return err
	}

	st := store.NewStore(pool)
	maker, err := token.NewJWTMaker(cfg.JWTSecret)
	if err != nil {
		return err
	}
	mailer, err := mail.NewSMTPMailer(cfg)
	if err != nil {
		return err
	}
	riverClient, err := worker.NewClient(ctx, pool, cfg.RiverMaxWorkers, st, mailer, "http://localhost"+cfg.HTTPAddr)
	if err != nil {
		return err
	}

	server, err := api.NewServer(cfg, st, maker, riverClient)
	if err != nil {
		return err
	}

	// Wire readyz to a real DB ping now that the pool is available.
	e := server.Handler()
	e.GET("/readyz", func(c *echo.Context) error {
		pingCtx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})

	sc := echo.StartConfig{Address: cfg.HTTPAddr, GracefulTimeout: 10 * time.Second}
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runWorker(ctx context.Context, cmd *cli.Command) error {
	cfg, err := mustConfig(cmd)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DBSource)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := runMigrations(ctx, pool); err != nil {
		return err
	}

	st := store.NewStore(pool)
	mailer, err := mail.NewSMTPMailer(cfg)
	if err != nil {
		return err
	}
	riverClient, err := worker.NewClient(ctx, pool, cfg.RiverMaxWorkers, st, mailer, "http://localhost"+cfg.HTTPAddr)
	if err != nil {
		return err
	}

	if err := riverClient.Start(ctx); err != nil {
		return err
	}
	slog.Info("worker started")

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return riverClient.Stop(shutdownCtx)
}
```

Note: re-registering `/readyz` may conflict with the route registered in `registerRoutes`. To avoid a duplicate-route panic, remove `s.router.GET("/readyz", s.readyz)` from `registerRoutes` and delete the placeholder `readyz` handler in `server.go`; keep `/readyz` wired only here where the pool is available. Verify `pool.Ping`, `stdlib.OpenDBFromPool`, and River `Start`/`Stop`/`Insert` names against installed versions.

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: builds with no errors. Resolve any remaining API-name mismatches.

- [ ] **Step 3: Run unit tests and lint**

Run: `go test -race ./... && mise run golangci-lint`
Expected: unit tests PASS; lint clean (fix issues if any).

- [ ] **Step 4: Commit**

```bash
git add cmd/app/main.go internal/db/migrate.go internal/api/server.go internal/api/routes.go
git commit -m "feat: wire serve and worker subcommands with startup migrations"
```

---

### Task 15: Auth rate limiting and API handler test

**Files:**
- Modify: `internal/api/routes.go`
- Create: `internal/api/user_test.go`

**Interfaces:**
- Consumes: `Server`, Echo rate limiter middleware.
- Produces: per-IP rate limiting on auth routes; a handler test proving `createUser` validates input and returns 400 on bad body without touching the DB.

- [ ] **Step 1: Add rate limiter to auth-sensitive routes in `routes.go`**

Add import `"github.com/labstack/echo/v5/middleware"` and wrap the login/renew routes:
```go
	authLimiter := middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(5)))
	v1.POST("/users/login", s.loginUser, authLimiter)
	v1.POST("/tokens/renew", s.renewToken, authLimiter)
```
Add import `"golang.org/x/time/rate"`. Remove the earlier unqualified `v1.POST("/users/login", ...)`/`/tokens/renew` registrations so each route is registered once.

Note: confirm `middleware.RateLimiter` / `NewRateLimiterMemoryStore` exist in Echo v5.3.0 (the Rate Limiter middleware is listed in the v5 middleware set). If the constructor signature differs, adjust to the documented one.

- [ ] **Step 2: Write `user_test.go`**

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vancanhuit/simplebank/internal/config"
	"github.com/vancanhuit/simplebank/internal/token"
)

type stubStore struct{ store.Store }

func newTestServer(t *testing.T) *Server {
	t.Helper()
	maker, err := token.NewJWTMaker("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(config.Config{JWTSecret: "01234567890123456789012345678901"}, nil, maker, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreateUserBadRequest(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"username":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}
```

Note: remove the unused `stubStore` if not needed, or keep it as scaffolding for future handler tests. Add the `store` import (`store "github.com/vancanhuit/simplebank/internal/db"`) only if `stubStore` is retained. The test relies on validation failing before any store call, so `store` may be `nil`.

- [ ] **Step 3: Run test**

Run: `go test ./internal/api/ -run TestCreateUserBadRequest -v`
Expected: PASS (returns 400 from validation).

- [ ] **Step 4: Commit**

```bash
git add internal/api/routes.go internal/api/user_test.go
git commit -m "feat: add auth rate limiting and user handler validation test"
```

---

### Task 16: Update compose, mise, and Dockerfile for full app

**Files:**
- Modify: `compose.yaml`
- Modify: `mise.toml`
- Modify: `Dockerfile`

**Interfaces:**
- Consumes: the built binary and its subcommands.
- Produces: `app-dev` env wired to Postgres/Mailpit; migrations run automatically on `serve` startup; Docker build copying `internal/`.

- [ ] **Step 1: Add env + command to `app-dev` in `compose.yaml`**

Replace the `app-dev` service with:
```yaml
  app-dev:
    build: .
    command: ["serve"]
    environment:
      HTTP_ADDR: ":8080"
      DB_SOURCE: "postgres://simplebank_dev:simplebank_dev@postgres-dev:5432/simplebank_dev?sslmode=disable"
      JWT_SECRET: "0123456789012345678901234567890123456789"
      SMTP_HOST: "mailpit-dev"
      SMTP_PORT: "1025"
      SMTP_FROM: "no-reply@simplebank.local"
    ports:
      - "8080:8080"
    networks:
      - dev
    depends_on:
      postgres-dev:
        condition: service_healthy
    profiles:
      - dev
```

- [ ] **Step 2: Update Dockerfile to copy `internal/`**

In `Dockerfile`, add `COPY internal internal` immediately after `COPY cmd cmd`.

- [ ] **Step 3: Verify Docker build**

Run: `docker compose --profile dev build app-dev`
Expected: image builds (Go build succeeds inside the container).

- [ ] **Step 4: Verify migrations run on `serve` startup against dev DB**

Run:
```bash
docker compose --profile dev up -d --wait postgres-dev
DB_SOURCE="postgres://simplebank_dev:simplebank_dev@localhost:5432/simplebank_dev?sslmode=disable" \
  JWT_SECRET="0123456789012345678901234567890123456789" SMTP_FROM="a@b.c" SMTP_HOST="localhost" \
  go run ./cmd/app serve
```
Expected: logs "domain migrations applied" and "river migrations applied", then the server starts listening. Stop with Ctrl-C; tear down with `docker compose --profile dev down -v`.

- [ ] **Step 5: Commit**

```bash
git add compose.yaml mise.toml Dockerfile
git commit -m "chore: wire app service env and docker build for internal"
```

---

### Task 17: Final verification pass

**Files:**
- None (verification only).

- [ ] **Step 1: Build, vet, lint, unit tests**

Run: `go build ./... && go vet ./... && mise run golangci-lint && go test -race -cover ./...`
Expected: all succeed.

- [ ] **Step 2: Integration tests**

Run: `mise run test:integration`
Expected: store + TransferTx concurrency tests PASS against the `test` profile.

- [ ] **Step 3: Vulnerability check**

Run: `mise run govulncheck`
Expected: no known vulnerabilities (or only advisory notes).

- [ ] **Step 4: Commit any fixups**

```bash
git add -A
git commit -m "chore: final verification fixups"
```

## Self-Review

**Spec coverage:**
- Users/Accounts/Entries/Transfers/Sessions/VerifyEmails schema → Task 4; queries → Task 5.
- TransferTx atomic + deterministic lock order + retryable/insufficient-balance → Task 7; concurrency test → Task 8.
- JWT auth (access+refresh, sessions, echo-jwt) → Tasks 9, 13.
- Async email via River transactional enqueue + worker + go-mail → Tasks 10, 11, 13 (enqueue in createUser).
- Config/CLI (urfave/cli v3, env sources, subcommands) → Tasks 2, 14.
- REST `/api/v1`, health probes at root, error mapping → Tasks 12, 13, 14.
- Security hardening (bcrypt≥12, rate limiting, validation, parameterized queries) → Tasks 3, 13, 15.
- Ops (slog, layered timeouts, graceful shutdown, livez/readyz) → Tasks 12, 14.
- Container wiring → Task 16.

**Known verification points** flagged inline for the implementer (exact library API names may differ across installed minor versions): pgx `stdlib.OpenDBFromPool`; go-mail option/method names; River `NewClient`/`Config`/`AddWorker`/`Insert`/`Start`/`Stop`/`rivermigrate` API; Echo `middleware.RateLimiter` constructor; urfave/cli `Int` return type. Each has a note to adjust to the installed version while keeping the documented interfaces stable.
