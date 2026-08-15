package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type Store interface {
	sqlcdb.Querier
	TransferTx(ctx context.Context, arg TransferTxParams) (TransferTxResult, error)
	CreateUserTx(ctx context.Context, arg CreateUserTxParams) (sqlcdb.User, error)
	VerifyEmailTx(ctx context.Context, arg VerifyEmailTxParams) (VerifyEmailTxResult, error)
	CreateAccountTx(ctx context.Context, arg sqlcdb.CreateAccountParams) (sqlcdb.Account, error)
	ReconcileAccount(ctx context.Context, id uuid.UUID) (Reconciliation, error)
	RotateSessionTx(ctx context.Context, arg RotateSessionTxParams) (sqlcdb.Session, error)
}

type SQLStore struct {
	*sqlcdb.Queries
	connPool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) Store {
	return &SQLStore{
		Queries:  sqlcdb.New(pool),
		connPool: pool,
	}
}

func (s *SQLStore) execTx(ctx context.Context, fn func(*sqlcdb.Queries) error) error {
	return pgx.BeginFunc(ctx, s.connPool, func(tx pgx.Tx) error {
		return fn(sqlcdb.New(tx))
	})
}
