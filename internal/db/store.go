package store

import (
	"context"
	"uuid"

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
	RotateSessionTx(ctx context.Context, arg RotateSessionTxParams) (sqlcdb.Session, error)
	ListNotificationsPage(ctx context.Context, arg ListNotificationsPageParams) (ListNotificationsPageResult, error)
	MarkNotificationReadTx(ctx context.Context, owner string, id uuid.UUID) (int64, error)
	MarkAllNotificationsReadTx(ctx context.Context, owner string) (int64, error)
}

type SQLStore struct {
	*sqlcdb.Queries
	connPool                  *pgxpool.Pool
	afterListNotifications    func()
	afterMarkNotificationRead func()
}

func New(pool *pgxpool.Pool) Store {
	return &SQLStore{
		Queries:  sqlcdb.New(pool),
		connPool: pool,
	}
}

func (s *SQLStore) execTx(ctx context.Context, fn func(*sqlcdb.Queries) error) error {
	return s.execTxOptions(ctx, pgx.TxOptions{}, fn)
}

func (s *SQLStore) execTxOptions(
	ctx context.Context,
	opts pgx.TxOptions,
	fn func(*sqlcdb.Queries) error,
) error {
	return pgx.BeginTxFunc(ctx, s.connPool, opts, func(tx pgx.Tx) error {
		return fn(sqlcdb.New(tx))
	})
}
