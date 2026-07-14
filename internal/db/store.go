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
