# Task 5: sqlc queries and generated code

**Files:**
- Create: `internal/db/query/users.sql`
- Create: `internal/db/query/accounts.sql`
- Create: `internal/db/query/entries.sql`
- Create: `internal/db/query/transfers.sql`
- Create: `internal/db/query/sessions.sql`
- Create: `internal/db/query/verify_emails.sql`
- Generated (do NOT hand-edit): `internal/db/sqlc/*.go`

## Produces (package `db` at `internal/db/sqlc`)
`Querier` interface + `Queries` struct with methods: `CreateUser`, `GetUser`, `VerifyUserEmail`, `CreateAccount`, `GetAccount`, `GetAccountForUpdate`, `ListAccounts`, `AddAccountBalance`, `CreateEntry`, `CreateTransfer`, `CreateSession`, `GetSession`, `CreateVerifyEmail`, `UpdateVerifyEmail`. Model structs `User`, `Account`, `Entry`, `Transfer`, `Session`, `VerifyEmail`. `type DBTX interface`, `func New(db DBTX) *Queries`, `(*Queries).WithTx(tx pgx.Tx) *Queries`.

## Step 1: `internal/db/query/users.sql`
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

## Step 2: `internal/db/query/accounts.sql`
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

## Step 3: `entries.sql`, `transfers.sql`, `sessions.sql`, `verify_emails.sql`
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

## Step 4: Generate sqlc code
Run: `mise run sqlc:generate` (installs sqlc, then runs `sqlc generate`).
Expected: files created under `internal/db/sqlc/` (`db.go`, `models.go`, `querier.go`, `*.sql.go`); `go build ./internal/db/sqlc/` passes.

## Step 5: Commit
```bash
git add internal/db/query/ internal/db/sqlc/
git commit -m "feat: add sqlc queries and generated db code"
```

## Global Constraints
- Money is `bigint` (Go `int64`); PKs are `uuid` (mapped to `github.com/google/uuid.UUID` via sqlc.yaml overrides), timestamps `time.Time`.
- Parameterized queries only (no string concatenation) — sqlc enforces this.
- Do NOT hand-edit generated files; if a query needs changing, edit the `.sql` and regenerate.
- Conventional commit message.

## Notes for you
- If `sqlc generate` reports issues with `sqlc.arg(...)` or `FOR NO KEY UPDATE`, ensure sqlc is a recent version (the mise task installs `@latest`). Report the exact sqlc version used.
- Record the generated method/param/field names you observe (e.g. `ListAccountsParams.Limit`/`Offset` types, `Session.ClientIp` vs `ClientIP`) in your report — later tasks depend on these exact names.

## Report contract
Write full report to `.superpowers/sdd/task-5-report.md`, INCLUDING the exact generated names for: `AddAccountBalanceParams`, `ListAccountsParams` (field names + types), `CreateSessionParams` (esp. the client IP field name), `UpdateVerifyEmailParams`, and the `Account`/`Session`/`VerifyEmail` struct field names. Return only: status, commit hash(es), one-line summary, sqlc version, and the key generated names.
