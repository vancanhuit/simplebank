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
