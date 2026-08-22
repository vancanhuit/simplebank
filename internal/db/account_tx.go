package store

import (
	"context"
	"uuid"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

// CreateAccountTx creates an account and, when it opens with a non-zero
// balance, posts a matching opening entry. Recording the opening deposit as an
// entry keeps the ledger invariant balance == SUM(entries) true from the start,
// which is what ReconcileAccount checks.
func (s *SQLStore) CreateAccountTx(ctx context.Context, arg sqlcdb.CreateAccountParams) (sqlcdb.Account, error) {
	var account sqlcdb.Account
	err := s.execTx(ctx, func(q *sqlcdb.Queries) error {
		var err error
		account, err = q.CreateAccount(ctx, arg)
		if err != nil {
			return ClassifyError(err)
		}
		if arg.Balance != 0 {
			if _, err = q.CreateEntry(ctx, sqlcdb.CreateEntryParams{
				AccountID: account.ID,
				Amount:    arg.Balance,
			}); err != nil {
				return ClassifyError(err)
			}
		}
		return nil
	})
	return account, err
}

// Reconciliation reports whether an account's stored balance matches the sum of
// its ledger entries. Balanced is false when they diverge, which signals
// corruption that needs investigation.
type Reconciliation struct {
	AccountID     uuid.UUID `json:"account_id"`
	StoredBalance int64     `json:"stored_balance"`
	LedgerBalance int64     `json:"ledger_balance"`
	Balanced      bool      `json:"balanced"`
}

// ReconcileAccount compares an account's stored balance against the sum of its
// entries. It is a read-only audit intended for background reconciliation jobs.
func (s *SQLStore) ReconcileAccount(ctx context.Context, id uuid.UUID) (Reconciliation, error) {
	account, err := s.GetAccount(ctx, id)
	if err != nil {
		return Reconciliation{}, ClassifyError(err)
	}
	ledger, err := s.GetAccountLedgerBalance(ctx, id)
	if err != nil {
		return Reconciliation{}, ClassifyError(err)
	}
	return Reconciliation{
		AccountID:     id,
		StoredBalance: account.Balance,
		LedgerBalance: ledger,
		Balanced:      account.Balance == ledger,
	}, nil
}
