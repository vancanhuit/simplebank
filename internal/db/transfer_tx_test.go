//go:build integration

package store

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/util"
)

func createTestUser(t *testing.T) sqlcdb.User {
	t.Helper()
	hashed, err := util.HashPassword(util.RandomString(8))
	if err != nil {
		t.Fatal(err)
	}
	user, err := testStore.CreateUser(t.Context(), sqlcdb.CreateUserParams{
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
	acc, err := testStore.CreateAccount(t.Context(), sqlcdb.CreateAccountParams{
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

	type outcome struct {
		res TransferTxResult
		err error
	}
	ch := make(chan outcome, n)

	for range n {
		go func() {
			res, err := testStore.TransferTx(t.Context(), TransferTxParams{
				FromAccountID: acc1.ID,
				ToAccountID:   acc2.ID,
				Amount:        amount,
			})
			ch <- outcome{res, err}
		}()
	}

	seen := make(map[uuid.UUID]bool)
	for range n {
		o := <-ch
		if o.err != nil {
			t.Fatalf("transfer failed: %v", o.err)
		}
		res := o.res

		if res.Transfer.ID == uuid.Nil {
			t.Fatal("transfer id should be set")
		}
		if seen[res.Transfer.ID] {
			t.Fatalf("duplicate transfer id %s", res.Transfer.ID)
		}
		seen[res.Transfer.ID] = true

		if res.Transfer.FromAccountID != acc1.ID || res.Transfer.ToAccountID != acc2.ID {
			t.Errorf("transfer accounts = %s/%s, want %s/%s",
				res.Transfer.FromAccountID, res.Transfer.ToAccountID, acc1.ID, acc2.ID)
		}
		if res.Transfer.Amount != amount {
			t.Errorf("transfer amount = %d, want %d", res.Transfer.Amount, amount)
		}
		if res.FromAccount.ID != acc1.ID || res.ToAccount.ID != acc2.ID {
			t.Errorf("result accounts = %s/%s, want %s/%s",
				res.FromAccount.ID, res.ToAccount.ID, acc1.ID, acc2.ID)
		}
		if res.FromEntry.AccountID != acc1.ID || res.FromEntry.Amount != -amount {
			t.Errorf("from entry = {%s, %d}, want {%s, %d}",
				res.FromEntry.AccountID, res.FromEntry.Amount, acc1.ID, -amount)
		}
		if res.ToEntry.AccountID != acc2.ID || res.ToEntry.Amount != amount {
			t.Errorf("to entry = {%s, %d}, want {%s, %d}",
				res.ToEntry.AccountID, res.ToEntry.Amount, acc2.ID, amount)
		}
	}

	updated1, err := testStore.GetAccount(t.Context(), acc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated2, err := testStore.GetAccount(t.Context(), acc2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.Balance != 1000-int64(n)*amount {
		t.Errorf("acc1 balance = %d, want %d", updated1.Balance, 1000-int64(n)*amount)
	}
	if updated2.Balance != 1000+int64(n)*amount {
		t.Errorf("acc2 balance = %d, want %d", updated2.Balance, 1000+int64(n)*amount)
	}
}

// TestTransferTxDeadlockPrevention drives transfers in both directions
// concurrently so both branches of the UUID-ordered locking run. The net
// effect is zero, so both balances must return to their starting value.
func TestTransferTxDeadlockPrevention(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	n := 10 // even: half each direction, net zero
	amount := int64(10)
	errs := make(chan error, n)

	for i := range n {
		from, to := acc1.ID, acc2.ID
		if i%2 == 1 {
			from, to = acc2.ID, acc1.ID
		}
		go func() {
			_, err := testStore.TransferTx(t.Context(), TransferTxParams{
				FromAccountID: from,
				ToAccountID:   to,
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

	updated1, err := testStore.GetAccount(t.Context(), acc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated2, err := testStore.GetAccount(t.Context(), acc2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.Balance != 1000 {
		t.Errorf("acc1 balance = %d, want 1000", updated1.Balance)
	}
	if updated2.Balance != 1000 {
		t.Errorf("acc2 balance = %d, want 1000", updated2.Balance)
	}
}

func TestTransferTxInsufficientBalance(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	_, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID: acc1.ID,
		ToAccountID:   acc2.ID,
		Amount:        100000,
	})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("want ErrInsufficientBalance, got %v", err)
	}

	// The whole tx must roll back: both balances stay untouched.
	updated1, err := testStore.GetAccount(t.Context(), acc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated2, err := testStore.GetAccount(t.Context(), acc2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.Balance != 1000 {
		t.Errorf("acc1 balance changed after failed transfer: %d, want 1000", updated1.Balance)
	}
	if updated2.Balance != 1000 {
		t.Errorf("acc2 balance changed after failed transfer: %d, want 1000", updated2.Balance)
	}
}

// TestTransferTxForeignKeyViolation transfers to a non-existent account. The
// CreateTransfer insert must fail the FK constraint and roll the tx back,
// leaving the source balance untouched.
func TestTransferTxForeignKeyViolation(t *testing.T) {
	u1 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)

	_, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID: acc1.ID,
		ToAccountID:   uuid.New(), // no such account
		Amount:        10,
	})
	if !errors.Is(err, ErrForeignKeyViolation) {
		t.Fatalf("want ErrForeignKeyViolation, got %v", err)
	}

	updated1, err := testStore.GetAccount(t.Context(), acc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.Balance != 1000 {
		t.Errorf("acc1 balance changed after failed transfer: %d, want 1000", updated1.Balance)
	}
}

// TestTransferTxExactBalance drains the full balance. The guard must allow the
// balance to reach exactly zero (>= 0), not reject it (> 0).
func TestTransferTxExactBalance(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	res, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID: acc1.ID,
		ToAccountID:   acc2.ID,
		Amount:        1000, // entire balance
	})
	if err != nil {
		t.Fatalf("draining the full balance should succeed, got %v", err)
	}
	if res.FromAccount.Balance != 0 {
		t.Errorf("from account balance = %d, want 0", res.FromAccount.Balance)
	}
	if res.ToAccount.Balance != 2000 {
		t.Errorf("to account balance = %d, want 2000", res.ToAccount.Balance)
	}

	updated1, err := testStore.GetAccount(t.Context(), acc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.Balance != 0 {
		t.Errorf("persisted acc1 balance = %d, want 0", updated1.Balance)
	}
}

// TestTransferTxPersistsRows confirms the transfer and both entry rows are
// actually committed, not just returned from the RETURNING clause. There is no
// sqlc read query for these tables, so it asserts against the pool directly.
func TestTransferTxPersistsRows(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	amount := int64(25)
	res, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID: acc1.ID,
		ToAccountID:   acc2.ID,
		Amount:        amount,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()
	var (
		gotAmount int64
		from, to  uuid.UUID
	)
	err = testPool.QueryRow(ctx,
		`SELECT amount, from_account_id, to_account_id FROM transfers WHERE id = $1`,
		res.Transfer.ID).Scan(&gotAmount, &from, &to)
	if err != nil {
		t.Fatalf("transfer row not persisted: %v", err)
	}
	if gotAmount != amount || from != acc1.ID || to != acc2.ID {
		t.Errorf("persisted transfer = {%d, %s, %s}, want {%d, %s, %s}",
			gotAmount, from, to, amount, acc1.ID, acc2.ID)
	}

	var entryCount int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM entries WHERE id IN ($1, $2)`,
		res.FromEntry.ID, res.ToEntry.ID).Scan(&entryCount); err != nil {
		t.Fatal(err)
	}
	if entryCount != 2 {
		t.Errorf("persisted entries = %d, want 2", entryCount)
	}
}
