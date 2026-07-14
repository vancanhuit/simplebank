package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/vancanhuit/simplebank/internal/db/migrations"
)

// MigrateSchema applies pending domain migrations under a PostgreSQL
// session-level advisory lock, so concurrent replicas starting together
// serialize migration application safely.
func MigrateSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		sqlDB,
		migrations.FS,
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return err
	}

	_, err = provider.Up(ctx)
	return err
}
