# Task 6+7 Report: Store + TransferTx

**Status:** DONE
**Commit:** `61a7bf6ea54623195a3a5bc79a5ee3d493c3eb1d`

## Summary
- `feat: add store interface, execTx, error translation, and TransferTx`
- Test `TestClassifyError` PASS; `go build ./...` and `go vet ./...` clean.

## TDD Execution
1. Wrote `internal/db/errors_test.go` — ran `go test ./internal/db/ -run TestClassifyError -v` → FAIL (undefined: ClassifyError, ErrRecordNotFound, ErrUniqueViolation, ErrForeignKeyViolation) as expected.
2. Wrote `internal/db/errors.go`, `internal/db/store.go`, `internal/db/transfer_tx.go` verbatim per brief.
3. Re-ran test → PASS.
4. `go build ./...` → clean; `go vet ./...` → clean.
5. Committed the four files with the exact conventional message.

## Files Created
- `internal/db/errors.go` — app-level sentinel errors + `ClassifyError` (pgx.ErrNoRows → ErrRecordNotFound; 23505 → ErrUniqueViolation; 23503 → ErrForeignKeyViolation; pass-through otherwise).
- `internal/db/store.go` — `Store` interface (embeds `sqlcdb.Querier` + `TransferTx`), `SQLStore`, `NewStore`, `execTx` with rollback-on-error and rebind via `sqlcdb.New(tx)`.
- `internal/db/transfer_tx.go` — `TransferTxParams`/`TransferTxResult`, `TransferTx` with deterministic UUID lock ordering, `moveMoney`, `mapBalanceError` (no-row balance guard → ErrInsufficientBalance).
- `internal/db/errors_test.go` — table of ClassifyError mappings.

## Constraints Honored
- `pgx.Tx` never leaves `internal/db`; queries rebind via `sqlcdb.New(tx)`.
- Transaction callback is pure DB, no side effects.
- pgx errors translated to stable app errors at store boundary.
- Money int64, PKs uuid.UUID.
- No temporary `fmt` placeholder added.
- Generated names used verbatim (`AddAccountBalanceParams{ID, Amount}`, etc.).

## Concerns
None.
