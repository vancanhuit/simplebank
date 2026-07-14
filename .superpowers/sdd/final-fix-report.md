# Final-review fix report

Branch: `feat/simplebank-implementation` · Repo: simplebank

## Changes per fix

### FIX 1 (CRITICAL) — Atomic user creation + verify-email enqueue (transactional outbox)
- Added `CreateUserTx` and `CreateUserTxParams` in new `internal/db/user_tx.go` (package `store`). It opens a `pgx` transaction on `connPool`, inserts the user via `sqlcdb.New(tx).CreateUser`, runs the `AfterCreate(tx, user)` hook inside the same tx, and commits only if the hook succeeds (deferred rollback otherwise).
- Added `CreateUserTx(ctx, arg) (sqlcdb.User, error)` to the `Store` interface in `internal/db/store.go`.
- `internal/api/user.go` `createUser` now calls `s.store.CreateUserTx(...)` and, in `AfterCreate`, enqueues the job with `s.riverClient.InsertTx(ctx, tx, worker.SendVerifyEmailArgs{...}, nil)`. Added `github.com/jackc/pgx/v5` import.

**River InsertTx integration detail:** `riverClient` is `*river.Client[pgx.Tx]`. `InsertTx(ctx, tx, args, opts)` writes the River job row using the *same* `pgx.Tx` that inserted the user. Because both writes share one transaction, a failed enqueue (or any error from `AfterCreate`) triggers the deferred `tx.Rollback`, so the user row is never persisted without its verification job — a true transactional outbox.

### FIX 2 (IMPORTANT) — Atomic verifyEmail
- Added `VerifyEmailTx`, `VerifyEmailTxParams`, `VerifyEmailTxResult` in `internal/db/user_tx.go`, running `UpdateVerifyEmail` + `VerifyUserEmail` inside one `execTx`.
- Added `VerifyEmailTx` to the `Store` interface.
- `internal/api/user.go` `verifyEmail` now calls `s.store.VerifyEmailTx(ctx, ...)`; `store.ErrRecordNotFound` → 400 "invalid or expired verification link"; success → 200 `{"is_verified": true}`.

### FIX 3 (IMPORTANT) — Remove committed binary
- `git rm --cached app` (untracked the ~9.8MB binary).
- Appended `/app` to `.gitignore` (with a leading newline since the file previously ended without one). Existing `dist/` line preserved; file now ends with a newline.

### FIX 4 (IMPORTANT) — Rate-limit registration
- `internal/api/routes.go`: `POST /users` now uses the existing `authLimiter`, alongside login and tokens/renew. `verify_email` left unthrottled by design.

### FIX 5 (MINOR) — Don't leak raw store error strings
- `internal/api/errors.go`: added `clientMessage(err)` mapping classified store/token errors to stable generic messages. The non-500 branch of `errorHandler` now uses `clientMessage(err)` instead of `err.Error()`. The `*echo.HTTPError` fast-path and 500 path are unchanged.

### FIX 6 (test) — Prove the outbox is atomic
- Added `internal/db/user_tx_test.go` (build tag `integration`): `TestCreateUserTxRollbackOnAfterCreateError` asserts that when `AfterCreate` returns an error, the user is absent afterward (tx rolled back).

## Verify output
| Step | Command | Result |
|------|---------|--------|
| build | `go build ./...` | PASS (exit 0) |
| vet | `go vet ./...` | PASS (exit 0) |
| lint | `mise run golangci-lint` | PASS (0 issues) |
| unit | `go test -race ./...` | PASS — 5 packages with tests, 0 failures |
| integration | `mise run test:integration` | PASS — api(2), config(1), db(4 incl. rollback + transfer), token(3), util(3); 0 failures |

## Concerns
None.
