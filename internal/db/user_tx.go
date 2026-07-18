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

	if arg.AfterCreate != nil {
		if err := arg.AfterCreate(tx, user); err != nil {
			return user, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return user, err
	}
	return user, nil
}
