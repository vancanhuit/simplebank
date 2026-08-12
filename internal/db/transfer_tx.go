package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type TransferTxParams struct {
	FromAccountID uuid.UUID `json:"from_account_id"`
	ToAccountID   uuid.UUID `json:"to_account_id"`
	Amount        int64     `json:"amount"`
	// Currency is the caller's asserted currency for both accounts. It is
	// re-validated against the locked account rows inside the transaction so a
	// currency change between the API pre-check and the money move cannot slip
	// through (TOCTOU).
	Currency string `json:"currency"`
	// IdempotencyKey collapses retries of the same logical transfer onto a
	// single row: resending the same key returns the original transfer instead
	// of moving money again.
	IdempotencyKey uuid.UUID `json:"idempotency_key"`
	// DailyLimit caps the total outgoing amount from the source account over the
	// trailing 24h window, in minor units. Zero disables the check.
	DailyLimit int64 `json:"daily_limit"`
}

type TransferTxResult struct {
	Transfer    sqlcdb.Transfer `json:"transfer"`
	FromAccount sqlcdb.Account  `json:"from_account"`
	ToAccount   sqlcdb.Account  `json:"to_account"`
	FromEntry   sqlcdb.Entry    `json:"from_entry"`
	ToEntry     sqlcdb.Entry    `json:"to_entry"`
}

func (s *SQLStore) TransferTx(ctx context.Context, arg TransferTxParams) (TransferTxResult, error) {
	// Idempotency fast path: a completed transfer for this key is replayed
	// without touching balances.
	if existing, err := s.GetTransferBySourceAndIdempotencyKey(ctx, sqlcdb.GetTransferBySourceAndIdempotencyKeyParams{
		FromAccountID:  arg.FromAccountID,
		IdempotencyKey: arg.IdempotencyKey,
	}); err == nil {
		return s.replayTransfer(ctx, existing, arg)
	} else if classified := ClassifyError(err); !errors.Is(classified, ErrRecordNotFound) {
		return TransferTxResult{}, classified
	}

	var result TransferTxResult
	err := s.execTx(ctx, func(q *sqlcdb.Queries) error {
		// Lock and validate both accounts first, in a deterministic order, so
		// existence and currency are checked against the same rows the balances
		// are moved on and concurrent transfers cannot deadlock.
		fromAccount, toAccount, err := lockAccounts(ctx, q, arg.FromAccountID, arg.ToAccountID)
		if err != nil {
			return err
		}
		if fromAccount.Currency != arg.Currency || toAccount.Currency != arg.Currency {
			return ErrCurrencyMismatch
		}

		if arg.DailyLimit > 0 {
			since := time.Now().Add(-24 * time.Hour)
			spent, err := q.SumOutgoingTransfersSince(ctx, sqlcdb.SumOutgoingTransfersSinceParams{
				FromAccountID: arg.FromAccountID,
				Since:         since,
			})
			if err != nil {
				return ClassifyError(err)
			}
			if spent+arg.Amount > arg.DailyLimit {
				return ErrDailyLimitExceeded
			}
		}

		result.Transfer, err = q.CreateTransfer(ctx, sqlcdb.CreateTransferParams{
			FromAccountID:  arg.FromAccountID,
			ToAccountID:    arg.ToAccountID,
			Amount:         arg.Amount,
			IdempotencyKey: arg.IdempotencyKey,
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

		// The balance guard (balance + amount >= 0) and bigint overflow are both
		// enforced by the UPDATE; a failed guard returns no row (insufficient
		// balance), an overflow returns a numeric-out-of-range error.
		result.FromAccount, err = q.AddAccountBalance(ctx, sqlcdb.AddAccountBalanceParams{
			ID:     arg.FromAccountID,
			Amount: -arg.Amount,
		})
		if err != nil {
			return mapBalanceError(err)
		}
		result.ToAccount, err = q.AddAccountBalance(ctx, sqlcdb.AddAccountBalanceParams{
			ID:     arg.ToAccountID,
			Amount: arg.Amount,
		})
		if err != nil {
			return mapBalanceError(err)
		}
		return nil
	})

	// A concurrent request carrying the same key can win the CreateTransfer race;
	// its unique-constraint violation is resolved by replaying the winner.
	if errors.Is(err, ErrUniqueViolation) {
		if existing, getErr := s.GetTransferBySourceAndIdempotencyKey(ctx, sqlcdb.GetTransferBySourceAndIdempotencyKeyParams{
			FromAccountID:  arg.FromAccountID,
			IdempotencyKey: arg.IdempotencyKey,
		}); getErr == nil {
			return s.replayTransfer(ctx, existing, arg)
		}
	}

	return result, err
}

// lockAccounts takes row locks on both accounts in a deterministic (smaller
// UUID first) order to avoid deadlocks between opposing transfers, then returns
// them keyed back to the caller's from/to roles.
func lockAccounts(
	ctx context.Context,
	q *sqlcdb.Queries,
	fromID, toID uuid.UUID,
) (fromAccount, toAccount sqlcdb.Account, err error) {
	if fromID.String() < toID.String() {
		if fromAccount, err = q.GetAccountForUpdate(ctx, fromID); err != nil {
			return fromAccount, toAccount, ClassifyError(err)
		}
		if toAccount, err = q.GetAccountForUpdate(ctx, toID); err != nil {
			return fromAccount, toAccount, ClassifyError(err)
		}
		return fromAccount, toAccount, nil
	}
	if toAccount, err = q.GetAccountForUpdate(ctx, toID); err != nil {
		return fromAccount, toAccount, ClassifyError(err)
	}
	if fromAccount, err = q.GetAccountForUpdate(ctx, fromID); err != nil {
		return fromAccount, toAccount, ClassifyError(err)
	}
	return fromAccount, toAccount, nil
}

// replayTransfer reconstructs a result for an already-committed transfer after
// validating the request-bound immutable fields. The balances reflect the
// accounts' current state; the per-transfer entries are not re-read because a
// replay posts nothing new.
func (s *SQLStore) replayTransfer(
	ctx context.Context,
	existing sqlcdb.Transfer,
	arg TransferTxParams,
) (TransferTxResult, error) {
	// Verify all request-bound immutable fields match the existing transfer.
	// If the caller is reusing the same key for a different transfer, reject
	// it as a conflict instead of silently returning the wrong transfer.
	if existing.FromAccountID != arg.FromAccountID ||
		existing.ToAccountID != arg.ToAccountID ||
		existing.Amount != arg.Amount {
		return TransferTxResult{}, ErrIdempotencyConflict
	}

	fromAccount, err := s.GetAccount(ctx, existing.FromAccountID)
	if err != nil {
		return TransferTxResult{}, ClassifyError(err)
	}
	toAccount, err := s.GetAccount(ctx, existing.ToAccountID)
	if err != nil {
		return TransferTxResult{}, ClassifyError(err)
	}

	// Currency is not stored on the transfer, but it must match the accounts.
	if fromAccount.Currency != arg.Currency || toAccount.Currency != arg.Currency {
		return TransferTxResult{}, ErrIdempotencyConflict
	}

	return TransferTxResult{
		Transfer:    existing,
		FromAccount: fromAccount,
		ToAccount:   toAccount,
	}, nil
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
