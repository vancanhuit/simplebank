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
