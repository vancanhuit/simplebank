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
	err := pgx.BeginFunc(ctx, s.connPool, func(tx pgx.Tx) error {
		q := sqlcdb.New(tx)
		var err error
		user, err = q.CreateUser(ctx, arg.CreateUserParams)
		if err != nil {
			return ClassifyError(err)
		}

		if arg.AfterCreate != nil {
			return arg.AfterCreate(tx, user)
		}
		return nil
	})
	return user, err
}
