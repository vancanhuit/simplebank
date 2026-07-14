# Task 8 Report: Store integration tests (migrations + TransferTx concurrency)

## Status: DONE

## Files created
- `internal/db/main_test.go` (`//go:build integration`) — `TestMain` boots pgxpool, runs goose migrations from embedded FS, builds `testStore`.
- `internal/db/transfer_tx_test.go` (`//go:build integration`) — `TestTransferTxConcurrent`, `TestTransferTxInsufficientBalance`, plus `createTestUser`/`createTestAccount` helpers.

## pgx handle path used
**Primary path: `stdlib.OpenDBFromPool(pool)`.**
Verified it exists in the installed `github.com/jackc/pgx/v5@v5.10.0` at
`stdlib/sql.go:237 func OpenDBFromPool(pool *pgxpool.Pool, opts ...OptionOpenDB) *sql.DB`.
The `sql.Open("pgx", dsn)` fallback was NOT needed.

## Compatibility verification
- Package name is `store` (matches). `NewStore(pool *pgxpool.Pool) Store` returns the `Store` interface which embeds `sqlcdb.Querier`, so `testStore.CreateUser/.CreateAccount/.GetAccount` resolve.
- Generated field names match Task 5: `CreateUserParams{Username, HashedPassword, FullName, Email}`, `CreateAccountParams{Owner, Balance, Currency}`, `Account.ID` (uuid.UUID), `Account.Balance` (int64).
- `ErrInsufficientBalance` lives in `internal/db/errors.go`; `util.HashPassword`, `util.RandomString`, `util.RandomOwner`, `util.USD` all present.
- No source adjustments to the brief's verbatim code were required.

## mise.toml change (exact)
In `[tasks."test:integration"]`, the `run` line changed from:
```
run = 'go test -race -cover -v ./cmd/... -tags=integration'
```
to:
```
run = 'go test -race -cover -v ./... -tags=integration'
```
`depends`/`depends_post` (compose test profile up/down) unchanged. DB_SOURCE relies on the default DSN baked into `main_test.go`
(`postgres://simplebank_test:simplebank_test@localhost:5433/simplebank_test?sslmode=disable`), which matches the compose `postgres-test` service.

## Test run summary
`mise run test:integration` brought up the `test` compose profile (postgres-test :5433 + mailpit-test), ran `go test -race -cover -v ./... -tags=integration`, then tore the profile down.

Package results:
- `internal/config` — PASS (77.8%)
- `internal/db` — PASS (83.3%): `TestClassifyError`, `TestTransferTxConcurrent` (7.32s), `TestTransferTxInsufficientBalance` (7.24s)
- `internal/util` — PASS (84.6%): `TestIsSupportedCurrency`, `TestPassword`
- `cmd/app`, `internal/db/sqlc` — no test-covered statements; `internal/db/migrations` — no test files

Pass counts: **6 test functions run, 6 PASS, 0 FAIL** (goose migration to version 1 succeeded). Run with `-race`, no data races reported.

## Concerns
None.
