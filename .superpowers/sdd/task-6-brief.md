# Task 6+7: Store (interface, execTx, error translation) + TransferTx

These two plan tasks are implemented together because the `store` package only
compiles once `TransferTx` exists (the `Store` interface embeds it and
`NewStore` returns `Store`). Do NOT add any temporary `fmt` placeholder.

**Files:**
- Create: `internal/db/errors.go`
- Create: `internal/db/store.go`
- Create: `internal/db/transfer_tx.go`
- Test: `internal/db/errors_test.go`

**Package name:** `store` (all files in `internal/db/` use `package store`).
**Import the generated sqlc package as:** `sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"` (its package name is `db`).

## Confirmed generated names (from Task 5, sqlc v1.31.1) — use verbatim
- `AddAccountBalanceParams{ Amount int64; ID uuid.UUID }`, returns `sqlcdb.Account`.
- `CreateTransferParams{ FromAccountID, ToAccountID uuid.UUID; Amount int64 }`.
- `CreateEntryParams{ AccountID uuid.UUID; Amount int64 }`.
- Models `sqlcdb.Transfer`, `sqlcdb.Account`, `sqlcdb.Entry`.

## Step 1: Write failing test `internal/db/errors_test.go`
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

## Step 2: Run test, verify FAIL
`go test ./internal/db/ -run TestClassifyError -v` → FAIL (undefined symbols).

## Step 3: Write `internal/db/errors.go`
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

## Step 4: Write `internal/db/store.go` (NO fmt placeholder)
```go
package store

import (
	"context"

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
```

## Step 5: Write `internal/db/transfer_tx.go`
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

## Step 6: Run test + build, verify PASS
- `go test ./internal/db/ -run TestClassifyError -v` → PASS.
- `go build ./...` and `go vet ./...` → clean.

## Step 7: Commit
```bash
git add internal/db/errors.go internal/db/store.go internal/db/transfer_tx.go internal/db/errors_test.go
git commit -m "feat: add store interface, execTx, error translation, and TransferTx"
```

## Global Constraints
- `pgx.Tx` must never leave `internal/db`; sqlc queries rebind via `sqlcdb.New(tx)`.
- Transaction callback is pure DB — no side effects.
- pgx errors translated to stable app errors at the store boundary.
- Money is int64; PKs uuid.UUID.
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-6-report.md`. Return only: status, commit hash(es), one-line test/build summary, concerns.
