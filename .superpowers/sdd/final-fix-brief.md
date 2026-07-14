# Final-review fixes (one subagent, all findings)

Repo: /home/canhdinh/workspace/simplebank, branch feat/simplebank-implementation.
Apply ALL of the following. Do NOT run `go mod tidy`.

## FIX 1 (CRITICAL) — Atomic user creation + verify-email enqueue (transactional outbox)
Currently `internal/api/user.go` `createUser` calls `s.store.CreateUser(...)` then `s.riverClient.Insert(...)` as TWO separate operations. Make them ONE database transaction using River's `InsertTx` (confirmed signature: `func (c *river.Client[pgx.Tx]) InsertTx(ctx, tx pgx.Tx, args JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)`; when the tx rolls back, the job insert rolls back too).

### 1a. Add `CreateUserTx` to the store (`internal/db/`)
Add to a store file (e.g. new `internal/db/user_tx.go`, package `store`), importing `github.com/jackc/pgx/v5` and `sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"`:
```go
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type CreateUserTxParams struct {
	sqlcdb.CreateUserParams
	// AfterCreate runs inside the same transaction as the user insert.
	// Use it to enqueue the verification job atomically (River InsertTx).
	// If it returns an error, the whole transaction rolls back.
	AfterCreate func(tx pgx.Tx, user sqlcdb.User) error
}

func (s *SQLStore) CreateUserTx(ctx context.Context, arg CreateUserTxParams) (sqlcdb.User, error) {
	var user sqlcdb.User

	tx, err := s.connPool.Begin(ctx)
	if err != nil {
		return user, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := sqlcdb.New(tx)
	user, err = q.CreateUser(ctx, arg.CreateUserParams)
	if err != nil {
		return user, ClassifyError(err)
	}

	if err := arg.AfterCreate(tx, user); err != nil {
		return user, err
	}

	if err := tx.Commit(ctx); err != nil {
		return user, err
	}
	return user, nil
}
```
Add `CreateUserTx(ctx context.Context, arg CreateUserTxParams) (sqlcdb.User, error)` to the `Store` interface in `internal/db/store.go`.

### 1b. Update `createUser` handler (`internal/api/user.go`)
Replace the CreateUser + Insert block with:
```go
	ctx := c.Request().Context()
	user, err := s.store.CreateUserTx(ctx, store.CreateUserTxParams{
		CreateUserParams: sqlcdb.CreateUserParams{
			Username:       req.Username,
			HashedPassword: hashed,
			FullName:       req.FullName,
			Email:          req.Email,
		},
		AfterCreate: func(tx pgx.Tx, u sqlcdb.User) error {
			_, err := s.riverClient.InsertTx(ctx, tx, worker.SendVerifyEmailArgs{Username: u.Username}, nil)
			return err
		},
	})
	if err != nil {
		return store.ClassifyError(err)
	}

	return c.JSON(http.StatusCreated, newUserResponse(user))
```
Add `"github.com/jackc/pgx/v5"` to user.go imports.

## FIX 2 (IMPORTANT) — Atomic verifyEmail
Add `VerifyEmailTx` to the store and use it so `UpdateVerifyEmail` + `VerifyUserEmail` run in one `execTx`.

In `internal/db/user_tx.go` (or another store file):
```go
import "github.com/google/uuid" // add to imports

type VerifyEmailTxParams struct {
	ID         uuid.UUID
	SecretCode string
}

type VerifyEmailTxResult struct {
	User        sqlcdb.User
	VerifyEmail sqlcdb.VerifyEmail
}

func (s *SQLStore) VerifyEmailTx(ctx context.Context, arg VerifyEmailTxParams) (VerifyEmailTxResult, error) {
	var res VerifyEmailTxResult
	err := s.execTx(ctx, func(q *sqlcdb.Queries) error {
		ve, err := q.UpdateVerifyEmail(ctx, sqlcdb.UpdateVerifyEmailParams{
			ID:         arg.ID,
			SecretCode: arg.SecretCode,
		})
		if err != nil {
			return ClassifyError(err)
		}
		res.VerifyEmail = ve

		u, err := q.VerifyUserEmail(ctx, ve.Username)
		if err != nil {
			return ClassifyError(err)
		}
		res.User = u
		return nil
	})
	return res, err
}
```
Add `VerifyEmailTx` to the `Store` interface. Update `internal/api/user.go` `verifyEmail` to call `s.store.VerifyEmailTx(ctx, store.VerifyEmailTxParams{ID: id, SecretCode: code})`; on `store.ErrRecordNotFound` return the existing 400 "invalid or expired verification link"; on success return the same 200 body.

## FIX 3 (IMPORTANT) — Remove committed binary
`app` (a ~9.8MB compiled binary) is tracked in git. Remove it from tracking and ignore it:
```bash
git rm --cached app
printf '/app\n' >> .gitignore
```
(Keep the existing `dist/` line; ensure `.gitignore` ends with a newline.)

## FIX 4 (IMPORTANT) — Rate-limit registration
In `internal/api/routes.go`, apply the existing `authLimiter` to `POST /users` as well:
```go
	v1.POST("/users", s.createUser, authLimiter)
```
Keep it on login and tokens/renew too. (Leave verify_email as-is; the 32-byte random code makes brute force infeasible.)

## FIX 5 (MINOR) — Don't leak raw store error strings to clients
In `internal/api/errors.go`, the non-500 branch currently returns `err.Error()` (so clients see "unique constraint violation" etc.). Map classified store errors to stable, generic client messages while still using the correct status. Add a helper:
```go
func clientMessage(err error) string {
	switch {
	case errors.Is(err, store.ErrRecordNotFound):
		return "resource not found"
	case errors.Is(err, store.ErrUniqueViolation):
		return "resource already exists"
	case errors.Is(err, store.ErrForeignKeyViolation):
		return "related resource not found"
	case errors.Is(err, store.ErrInsufficientBalance):
		return "insufficient balance"
	case errors.Is(err, token.ErrExpiredToken):
		return "token has expired"
	case errors.Is(err, token.ErrInvalidToken):
		return "token is invalid"
	default:
		return "request failed"
	}
}
```
In `errorHandler`, for the mapped (non-HTTPError, non-500) path use `clientMessage(err)` instead of `err.Error()`. Keep the `*echo.HTTPError` fast-path (its `Message` is already curated) and the 500 path (generic "internal server error" + log) unchanged.

## FIX 6 (test) — Prove the outbox is atomic
Add an integration test (build tag `integration`) in `internal/db/` — e.g. `user_tx_test.go` — asserting that when `AfterCreate` returns an error, the user row is NOT persisted (transaction rolled back):
```go
//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/util"
)

func TestCreateUserTxRollbackOnAfterCreateError(t *testing.T) {
	hashed, _ := util.HashPassword(util.RandomString(8))
	username := util.RandomOwner()
	boom := errors.New("enqueue failed")

	_, err := testStore.CreateUserTx(context.Background(), CreateUserTxParams{
		CreateUserParams: sqlcdb.CreateUserParams{
			Username:       username,
			HashedPassword: hashed,
			FullName:       util.RandomOwner(),
			Email:          util.RandomString(6) + "@example.com",
		},
		AfterCreate: func(tx pgx.Tx, user sqlcdb.User) error { return boom },
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want boom error, got %v", err)
	}

	// The user must NOT exist because the tx rolled back.
	if _, err := testStore.GetUser(context.Background(), username); !errors.Is(ClassifyError(err), ErrRecordNotFound) {
		t.Fatalf("expected user to be absent after rollback, got err=%v", err)
	}
}
```

## Verify
- `go build ./...`, `go vet ./...`, `mise run golangci-lint` → clean.
- `go test -race ./...` (unit) → pass.
- `mise run test:integration` → pass (incl. the new rollback test and existing TransferTx tests).

## Commit
Use focused commits or one combined commit:
```bash
git add -A
git commit -m "fix: make user creation and email enqueue atomic, harden api per review"
```
(The `git rm --cached app` is included in `git add -A` via the removal; ensure `app` is no longer tracked and `.gitignore` includes `/app`.)

## Report
Write the fix report to `.superpowers/sdd/final-fix-report.md` with: what changed per fix, the River InsertTx integration detail, and the full verify output (build/vet/lint/unit/integration pass counts). Return only: status, commit hash(es), one-line verify summary, concerns.
