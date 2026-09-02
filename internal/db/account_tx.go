package store

import (
	"context"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

// CreateAccountTx creates an account and, when it opens with a non-zero
// balance, posts a matching opening entry.
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
