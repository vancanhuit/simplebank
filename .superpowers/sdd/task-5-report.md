# Task 5 Report: sqlc queries and generated code

**Status:** DONE

## Summary
Created all six query files, ran `mise run sqlc:generate`, and verified both
`go build ./internal/db/sqlc/` and `go build ./...` succeed. No hand-editing of
generated files. `sqlc.arg(...)` and `FOR NO KEY UPDATE` worked without issue.

## sqlc version
**v1.31.1** (installed by the mise task via `@latest`).

## Files created (queries)
- `internal/db/query/users.sql` — CreateUser, GetUser, VerifyUserEmail
- `internal/db/query/accounts.sql` — CreateAccount, GetAccount, GetAccountForUpdate, ListAccounts, AddAccountBalance
- `internal/db/query/entries.sql` — CreateEntry
- `internal/db/query/transfers.sql` — CreateTransfer
- `internal/db/query/sessions.sql` — CreateSession, GetSession
- `internal/db/query/verify_emails.sql` — CreateVerifyEmail, UpdateVerifyEmail

## Files generated (do NOT hand-edit)
`internal/db/sqlc/`: `db.go`, `models.go`, `querier.go`, `accounts.sql.go`,
`entries.sql.go`, `sessions.sql.go`, `transfers.sql.go`, `users.sql.go`,
`verify_emails.sql.go`.

## Build verification
- `go build ./internal/db/sqlc/` → OK
- `go build ./...` → OK

## Generated infra (as expected)
`type DBTX interface`, `func New(db DBTX) *Queries`,
`func (q *Queries) WithTx(tx pgx.Tx) *Queries`, `Querier` interface
(emit_interface: true), JSON tags emitted, empty slices emitted.

## KEY GENERATED NAMES (later tasks depend on these EXACTLY)

### AddAccountBalanceParams
```go
type AddAccountBalanceParams struct {
	Amount int64     `json:"amount"`
	ID     uuid.UUID `json:"id"`
}
```
Returns `Account`. NOTE ordering: `Amount` first, then `ID`.

### ListAccountsParams
```go
type ListAccountsParams struct {
	Owner  string `json:"owner"`
	Limit  int32  `json:"limit"`
	Offset int32  `json:"offset"`
}
```
`Limit` and `Offset` are **int32** (not int64). Returns `[]Account`.

### CreateSessionParams (client IP field name)
```go
type CreateSessionParams struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	RefreshToken string    `json:"refresh_token"`
	UserAgent    string    `json:"user_agent"`
	ClientIp     string    `json:"client_ip"`
	IsBlocked    bool      `json:"is_blocked"`
	ExpiresAt    time.Time `json:"expires_at"`
}
```
Client IP field is **`ClientIp`** (lowercase `p`), NOT `ClientIP`.

### UpdateVerifyEmailParams
```go
type UpdateVerifyEmailParams struct {
	ID         uuid.UUID `json:"id"`
	SecretCode string    `json:"secret_code"`
}
```
Returns `VerifyEmail`.

### Model structs

**Account**
```go
type Account struct {
	ID        uuid.UUID `json:"id"`
	Owner     string    `json:"owner"`
	Balance   int64     `json:"balance"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
}
```

**Session**
```go
type Session struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	RefreshToken string    `json:"refresh_token"`
	UserAgent    string    `json:"user_agent"`
	ClientIp     string    `json:"client_ip"`   // ClientIp, not ClientIP
	IsBlocked    bool      `json:"is_blocked"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}
```

**VerifyEmail**
```go
type VerifyEmail struct {
	ID         uuid.UUID `json:"id"`
	Username   string    `json:"username"`
	Email      string    `json:"email"`
	SecretCode string    `json:"secret_code"`
	IsUsed     bool      `json:"is_used"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiredAt  time.Time `json:"expired_at"`
}
```

## Other useful generated names (for reference)
- `User`: ID, Username, HashedPassword, FullName, Email, IsEmailVerified, PasswordChangedAt, CreatedAt
- `Entry`: ID, AccountID, Amount, CreatedAt
- `Transfer`: ID, FromAccountID, ToAccountID, Amount, CreatedAt
- `CreateAccountParams`: Owner (string), Balance (int64), Currency (string)
- `CreateUserParams`: Username, HashedPassword, FullName, Email
- `CreateEntryParams`: AccountID (uuid.UUID), Amount (int64)
- `CreateTransferParams`: FromAccountID, ToAccountID (uuid.UUID), Amount (int64)
- `CreateVerifyEmailParams`: Username, Email, SecretCode
- `GetAccount`/`GetAccountForUpdate` take `id uuid.UUID` (scalar, not a params struct)
- `GetUser` takes `username string`; `VerifyUserEmail` takes `username string`
- `GetSession` takes `id uuid.UUID`

## Concerns
None.
