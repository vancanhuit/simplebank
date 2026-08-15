//go:build integration

package store

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/vancanhuit/simplebank/internal/currency"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/password"
	"github.com/vancanhuit/simplebank/internal/random"
)

func createTestUser(t *testing.T) sqlcdb.User {
	t.Helper()
	hashed, err := password.Hash(random.String(8))
	if err != nil {
		t.Fatal(err)
	}
	user, err := testStore.CreateUser(t.Context(), sqlcdb.CreateUserParams{
		Username:       random.Owner(),
		HashedPassword: hashed,
		FullName:       random.Owner(),
		Email:          random.String(6) + "@example.com",
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
		Currency: currency.USD,
	})
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

func createTestAccountWithBalance(t *testing.T, owner string, balance int64) sqlcdb.Account {
	t.Helper()
	account, err := testStore.CreateAccount(t.Context(), sqlcdb.CreateAccountParams{
		Owner: owner, Balance: balance, Currency: currency.USD,
	})
	if err != nil {
		t.Fatal(err)
	}
	return account
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
				FromAccountID:  acc1.ID,
				ToAccountID:    acc2.ID,
				Amount:         amount,
				Currency:       currency.USD,
				IdempotencyKey: uuid.New(),
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
				FromAccountID:  from,
				ToAccountID:    to,
				Amount:         amount,
				Currency:       currency.USD,
				IdempotencyKey: uuid.New(),
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
		FromAccountID:  acc1.ID,
		ToAccountID:    acc2.ID,
		Amount:         100000,
		Currency:       currency.USD,
		IdempotencyKey: uuid.New(),
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

// TestTransferTxMissingAccount transfers to a non-existent account. The
// in-transaction lock/validate step must fail with ErrRecordNotFound and roll
// the tx back, leaving the source balance untouched.
func TestTransferTxMissingAccount(t *testing.T) {
	u1 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)

	_, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  acc1.ID,
		ToAccountID:    uuid.New(), // no such account
		Amount:         10,
		Currency:       currency.USD,
		IdempotencyKey: uuid.New(),
	})
	if !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("want ErrRecordNotFound, got %v", err)
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
		FromAccountID:  acc1.ID,
		ToAccountID:    acc2.ID,
		Amount:         1000, // entire balance
		Currency:       currency.USD,
		IdempotencyKey: uuid.New(),
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
		FromAccountID:  acc1.ID,
		ToAccountID:    acc2.ID,
		Amount:         amount,
		Currency:       currency.USD,
		IdempotencyKey: uuid.New(),
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

func TestMonetaryRowsRejectUnsafeJavaScriptIntegers(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)
	ctx := t.Context()

	insertTransferAmount := func(t *testing.T, amount int64) error {
		t.Helper()
		tx, err := testPool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)

		_, err = tx.Exec(ctx, `
			INSERT INTO transfers (from_account_id, to_account_id, amount, idempotency_key)
			VALUES ($1, $2, $3, $4)`, acc1.ID, acc2.ID, amount, uuid.New())
		return err
	}
	insertEntryAmount := func(t *testing.T, amount int64) error {
		t.Helper()
		tx, err := testPool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)

		_, err = tx.Exec(ctx, `
			INSERT INTO entries (account_id, amount)
			VALUES ($1, $2)`, acc1.ID, amount)
		return err
	}
	requireCheckViolation := func(t *testing.T, err error, constraint string) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			t.Fatalf("want PostgreSQL check violation %s, got %v", constraint, err)
		}
		if pgErr.Code != "23514" || pgErr.ConstraintName != constraint {
			t.Fatalf("want check violation %s, got code=%s constraint=%s", constraint, pgErr.Code, pgErr.ConstraintName)
		}
	}

	if err := insertTransferAmount(t, currency.MaxSafeMinorUnits); err != nil {
		t.Fatalf("transfer amount at JavaScript-safe boundary should insert, got %v", err)
	}
	requireCheckViolation(t,
		insertTransferAmount(t, currency.MaxSafeMinorUnits+1),
		"transfers_amount_javascript_safe")

	if err := insertEntryAmount(t, currency.MaxSafeMinorUnits); err != nil {
		t.Fatalf("positive entry at JavaScript-safe boundary should insert, got %v", err)
	}
	if err := insertEntryAmount(t, -currency.MaxSafeMinorUnits); err != nil {
		t.Fatalf("negative entry at JavaScript-safe boundary should insert, got %v", err)
	}
	requireCheckViolation(t,
		insertEntryAmount(t, currency.MaxSafeMinorUnits+1),
		"entries_amount_javascript_safe")
	requireCheckViolation(t,
		insertEntryAmount(t, -currency.MaxSafeMinorUnits-1),
		"entries_amount_javascript_safe")
}

// TestTransferTxAllowsExactSafeDestinationBalance confirms that a transfer
// bringing the destination to exactly MaxSafeMinorUnits succeeds.
func TestTransferTxAllowsExactSafeDestinationBalance(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccountWithBalance(t, u2.Username, currency.MaxSafeMinorUnits-10)

	res, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  acc1.ID,
		ToAccountID:    acc2.ID,
		Amount:         10,
		Currency:       currency.USD,
		IdempotencyKey: uuid.New(),
	})
	if err != nil {
		t.Fatalf("transfer to exactly MaxSafeMinorUnits should succeed, got %v", err)
	}
	if res.ToAccount.Balance != currency.MaxSafeMinorUnits {
		t.Errorf("to account balance = %d, want %d", res.ToAccount.Balance, currency.MaxSafeMinorUnits)
	}
}

// TestTransferTxRejectsUnsafeDestinationBalance confirms that a transfer
// exceeding MaxSafeMinorUnits is rejected and both balances remain unchanged.
func TestTransferTxRejectsUnsafeDestinationBalance(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccountWithBalance(t, u2.Username, currency.MaxSafeMinorUnits-5)
	ctx := t.Context()

	var baselineTransferCount int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM transfers WHERE from_account_id = $1 AND to_account_id = $2`,
		acc1.ID, acc2.ID).Scan(&baselineTransferCount); err != nil {
		t.Fatal(err)
	}

	var baselineEntryCount int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM entries WHERE account_id IN ($1, $2)`,
		acc1.ID, acc2.ID).Scan(&baselineEntryCount); err != nil {
		t.Fatal(err)
	}

	idempKey := uuid.New()
	_, err := testStore.TransferTx(ctx, TransferTxParams{
		FromAccountID:  acc1.ID,
		ToAccountID:    acc2.ID,
		Amount:         10,
		Currency:       currency.USD,
		IdempotencyKey: idempKey,
	})
	if !errors.Is(err, ErrBalanceLimitExceeded) {
		t.Fatalf("want ErrBalanceLimitExceeded, got %v", err)
	}

	// Both balances must remain unchanged.
	updated1, err := testStore.GetAccount(t.Context(), acc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated2, err := testStore.GetAccount(t.Context(), acc2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.Balance != 1000 {
		t.Errorf("acc1 balance changed after rejected transfer: %d, want 1000", updated1.Balance)
	}
	if updated2.Balance != currency.MaxSafeMinorUnits-5 {
		t.Errorf("acc2 balance changed after rejected transfer: %d, want %d", updated2.Balance, currency.MaxSafeMinorUnits-5)
	}

	// No transfer row should have been created for the attempted idempotency key.
	_, err = testStore.GetTransferBySourceAndIdempotencyKey(ctx, sqlcdb.GetTransferBySourceAndIdempotencyKeyParams{
		FromAccountID:  acc1.ID,
		IdempotencyKey: idempKey,
	})
	if !errors.Is(ClassifyError(err), ErrRecordNotFound) {
		t.Errorf("transfer row should not exist for rejected transfer, got err: %v", err)
	}

	var transferCount int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM transfers WHERE from_account_id = $1 AND to_account_id = $2`,
		acc1.ID, acc2.ID).Scan(&transferCount); err != nil {
		t.Fatal(err)
	}
	if transferCount != baselineTransferCount {
		t.Errorf("persisted transfer rows = %d, want %d", transferCount, baselineTransferCount)
	}

	var entryCount int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM entries WHERE account_id IN ($1, $2)`,
		acc1.ID, acc2.ID).Scan(&entryCount); err != nil {
		t.Fatal(err)
	}
	if entryCount != baselineEntryCount {
		t.Errorf("persisted entry rows = %d, want %d", entryCount, baselineEntryCount)
	}
}
