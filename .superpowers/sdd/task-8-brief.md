# Task 8: Integration test for store (migrations + TransferTx concurrency)

**Files:**
- Create: `internal/db/main_test.go`
- Create: `internal/db/transfer_tx_test.go`

Both files are guarded with `//go:build integration`.

## Step 1: `internal/db/main_test.go`
```go
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

	testStore = NewStore(pool)
	os.Exit(m.Run())
}
```

COMPATIBILITY: verify `stdlib.OpenDBFromPool(pool)` exists in the installed pgx/v5. If it does NOT, replace those lines with opening a separate `database/sql` handle: `import ("database/sql"; _ "github.com/jackc/pgx/v5/stdlib")` then `sqlDB, err := sql.Open("pgx", dsn)` (handle the error). Keep goose using that `*sql.DB`. Report which path you took.

## Step 2: `internal/db/transfer_tx_test.go`
```go
//go:build integration

package store

import (
	"context"
	"testing"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/util"
)

func createTestUser(t *testing.T) sqlcdb.User {
	t.Helper()
	hashed, err := util.HashPassword(util.RandomString(8))
	if err != nil {
		t.Fatal(err)
	}
	user, err := testStore.CreateUser(context.Background(), sqlcdb.CreateUserParams{
		Username:       util.RandomOwner(),
		HashedPassword: hashed,
		FullName:       util.RandomOwner(),
		Email:          util.RandomString(6) + "@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func createTestAccount(t *testing.T, owner string) sqlcdb.Account {
	t.Helper()
	acc, err := testStore.CreateAccount(context.Background(), sqlcdb.CreateAccountParams{
		Owner:    owner,
		Balance:  1000,
		Currency: util.USD,
	})
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

func TestTransferTxConcurrent(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	n := 10
	amount := int64(10)
	errs := make(chan error, n)

	for range n {
		go func() {
			_, err := testStore.TransferTx(context.Background(), TransferTxParams{
				FromAccountID: acc1.ID,
				ToAccountID:   acc2.ID,
				Amount:        amount,
			})
			errs <- err
		}()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Fatalf("transfer failed: %v", err)
		}
	}

	updated1, _ := testStore.GetAccount(context.Background(), acc1.ID)
	updated2, _ := testStore.GetAccount(context.Background(), acc2.ID)
	if updated1.Balance != 1000-int64(n)*amount {
		t.Errorf("acc1 balance = %d, want %d", updated1.Balance, 1000-int64(n)*amount)
	}
	if updated2.Balance != 1000+int64(n)*amount {
		t.Errorf("acc2 balance = %d, want %d", updated2.Balance, 1000+int64(n)*amount)
	}
}

func TestTransferTxInsufficientBalance(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	_, err := testStore.TransferTx(context.Background(), TransferTxParams{
		FromAccountID: acc1.ID,
		ToAccountID:   acc2.ID,
		Amount:        100000,
	})
	if err != ErrInsufficientBalance {
		t.Fatalf("want ErrInsufficientBalance, got %v", err)
	}
}
```

COMPATIBILITY: verify the generated `CreateUserParams` / `CreateAccountParams` field names match (from Task 5: `CreateAccountParams{Owner, Balance, Currency}`, and `Account` has `ID`, `Balance`). Adjust if the generated names differ.

## Step 3: Run integration tests
Run: `mise run test:integration`
This starts the `test` compose profile (Postgres on :5433, Mailpit), runs `go test -race -cover -v ./cmd/... -tags=integration` per the existing mise task.

IMPORTANT: the existing `mise` `test:integration` task runs `./cmd/...` only. These tests live in `./internal/db/`. Update the `test:integration` run command in `mise.toml` to cover `./...` (or add `./internal/...`) so these tests actually run. Also ensure `DB_SOURCE` for the test DB is available to the test process — the existing compose `postgres-test` uses user/db `simplebank_test` on host port 5433; either export `DB_SOURCE=postgres://simplebank_test:simplebank_test@localhost:5433/simplebank_test?sslmode=disable` in the task or rely on the default DSN baked into `main_test.go` (which already matches). Prefer updating the mise task to run `go test -race -cover -v ./... -tags=integration` and keep the default DSN fallback.

## Step 4: Commit
```bash
git add internal/db/main_test.go internal/db/transfer_tx_test.go mise.toml
git commit -m "test: add store integration and TransferTx concurrency tests"
```

## Global Constraints
- Integration tests use build tag `integration` and the `test` compose profile.
- Tests run with `-race`.
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-8-report.md`, noting: which pgx handle path you used (OpenDBFromPool vs sql.Open), the exact `mise.toml` change, and the test run output summary (pass counts). Return only: status, commit hash(es), one-line test summary, concerns.
