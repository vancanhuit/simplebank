package store

import (
	"context"

	"github.com/google/uuid"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

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
