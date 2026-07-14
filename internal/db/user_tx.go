package store

import (
	"context"

	"github.com/google/uuid"
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

	if err := arg.AfterCreate(tx, user); err != nil {
		return user, err
	}

	if err := tx.Commit(ctx); err != nil {
		return user, err
	}
	return user, nil
}

type VerifyEmailTxParams struct {
	ID         uuid.UUID
	SecretCode string
}

type VerifyEmailTxResult struct {
	User        sqlcdb.User
	VerifyEmail sqlcdb.VerifyEmail
}

func (s *SQLStore) VerifyEmailTx(ctx context.Context, arg VerifyEmailTxParams) (VerifyEmailTxResult, error) {
	var res VerifyEmailTxResult
	err := s.execTx(ctx, func(q *sqlcdb.Queries) error {
		ve, err := q.UpdateVerifyEmail(ctx, sqlcdb.UpdateVerifyEmailParams{
			ID:         arg.ID,
			SecretCode: arg.SecretCode,
		})
		if err != nil {
			return ClassifyError(err)
		}
		res.VerifyEmail = ve

		u, err := q.VerifyUserEmail(ctx, ve.Username)
		if err != nil {
			return ClassifyError(err)
		}
		res.User = u
		return nil
	})
	return res, err
}
