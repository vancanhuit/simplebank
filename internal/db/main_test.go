//go:build integration

package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/vancanhuit/simplebank/internal/db/migrations"
)

var testStore Store

// testPool is the raw connection pool behind testStore, exposed so tests can
// assert persisted DB state directly for tables that have no sqlc read query.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	dsn := os.Getenv("DB_SOURCE")
	if dsn == "" {
		dsn = "postgres://simplebank_test:simplebank_test@localhost:5433/simplebank_test?sslmode=disable"
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		panic(err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		panic(err)
	}

	testPool = pool
	testStore = New(pool)
	os.Exit(m.Run())
}
