//go:build integration

package store

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/vancanhuit/simplebank/internal/currency"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

// TestTransferTxIdempotent replays the same idempotency key twice. The second
// call must not move money again: exactly one transfer row exists and the
// balances reflect a single movement.
func TestTransferTxIdempotent(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	key := uuid.New()
	amount := int64(100)
	arg := TransferTxParams{
		FromAccountID:  acc1.ID,
		ToAccountID:    acc2.ID,
		Amount:         amount,
		Currency:       currency.USD,
		IdempotencyKey: key,
	}

	first, err := testStore.TransferTx(t.Context(), arg)
	if err != nil {
		t.Fatalf("first transfer failed: %v", err)
	}
	second, err := testStore.TransferTx(t.Context(), arg)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}

	if first.Transfer.ID != second.Transfer.ID {
		t.Errorf("replay returned a different transfer: %s vs %s", first.Transfer.ID, second.Transfer.ID)
	}

	var count int
	if err := testPool.QueryRow(t.Context(),
		`SELECT count(*) FROM transfers WHERE idempotency_key = $1`, key).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("want exactly 1 transfer row for the key, got %d", count)
	}

	updated1, err := testStore.GetAccount(t.Context(), acc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.Balance != 1000-amount {
		t.Errorf("source debited twice: balance = %d, want %d", updated1.Balance, 1000-amount)
	}
}

// TestTransferTxConcurrentSameKey fires the same key from many goroutines. The
// unique constraint plus replay must collapse them to a single money movement.
func TestTransferTxConcurrentSameKey(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	key := uuid.New()
	amount := int64(100)
	n := 8
	errs := make(chan error, n)
	for range n {
		go func() {
			_, err := testStore.TransferTx(t.Context(), TransferTxParams{
				FromAccountID:  acc1.ID,
				ToAccountID:    acc2.ID,
				Amount:         amount,
				Currency:       currency.USD,
				IdempotencyKey: key,
			})
			errs <- err
		}()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent replay failed: %v", err)
		}
	}

	updated1, err := testStore.GetAccount(t.Context(), acc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.Balance != 1000-amount {
		t.Errorf("balance moved more than once: %d, want %d", updated1.Balance, 1000-amount)
	}
}

// TestTransferTxCurrencyMismatch feeds a currency that does not match the
// locked account rows. The tx must abort with ErrCurrencyMismatch.
func TestTransferTxCurrencyMismatch(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	_, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  acc1.ID,
		ToAccountID:    acc2.ID,
		Amount:         10,
		Currency:       currency.EUR, // accounts are USD
		IdempotencyKey: uuid.New(),
	})
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("want ErrCurrencyMismatch, got %v", err)
	}

	updated1, err := testStore.GetAccount(t.Context(), acc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.Balance != 1000 {
		t.Errorf("balance changed after mismatch: %d, want 1000", updated1.Balance)
	}
}

// TestTransferTxDailyLimit rejects a transfer once the trailing-window total
// would exceed the configured cap.
func TestTransferTxDailyLimit(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	// First transfer of 60 is under the 100 cap and succeeds.
	if _, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  acc1.ID,
		ToAccountID:    acc2.ID,
		Amount:         60,
		Currency:       currency.USD,
		IdempotencyKey: uuid.New(),
		DailyLimit:     100,
	}); err != nil {
		t.Fatalf("first transfer under the cap failed: %v", err)
	}

	// Second transfer of 60 would push the daily total to 120, over the cap.
	_, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  acc1.ID,
		ToAccountID:    acc2.ID,
		Amount:         60,
		Currency:       currency.USD,
		IdempotencyKey: uuid.New(),
		DailyLimit:     100,
	})
	if !errors.Is(err, ErrDailyLimitExceeded) {
		t.Fatalf("want ErrDailyLimitExceeded, got %v", err)
	}
}

// TestCreateAccountTxOpeningEntry checks that opening an account with a balance
// records a matching entry, keeping balance == SUM(entries).
func TestCreateAccountTxOpeningEntry(t *testing.T) {
	u := createTestUser(t)

	account, err := testStore.CreateAccountTx(t.Context(), sqlcdb.CreateAccountParams{
		Owner:    u.Username,
		Balance:  500,
		Currency: currency.USD,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec, err := testStore.ReconcileAccount(t.Context(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Balanced {
		t.Errorf("opening balance not reconciled: stored=%d ledger=%d", rec.StoredBalance, rec.LedgerBalance)
	}
}

// TestReconcileAccountAfterTransfer confirms the ledger stays balanced once
// funds move between two fully entry-backed accounts.
func TestReconcileAccountAfterTransfer(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1, err := testStore.CreateAccountTx(t.Context(), sqlcdb.CreateAccountParams{
		Owner: u1.Username, Balance: 1000, Currency: currency.USD,
	})
	if err != nil {
		t.Fatal(err)
	}
	acc2, err := testStore.CreateAccountTx(t.Context(), sqlcdb.CreateAccountParams{
		Owner: u2.Username, Balance: 0, Currency: currency.USD,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  acc1.ID,
		ToAccountID:    acc2.ID,
		Amount:         250,
		Currency:       currency.USD,
		IdempotencyKey: uuid.New(),
	}); err != nil {
		t.Fatal(err)
	}

	for _, id := range []uuid.UUID{acc1.ID, acc2.ID} {
		rec, err := testStore.ReconcileAccount(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if !rec.Balanced {
			t.Errorf("account %s unbalanced: stored=%d ledger=%d", id, rec.StoredBalance, rec.LedgerBalance)
		}
	}
}

func TestTransferTxSameKeyDifferentSourceDoesNotReplay(t *testing.T) {
	firstOwner := createTestUser(t)
	secondOwner := createTestUser(t)
	recipient := createTestUser(t)
	firstSource := createTestAccount(t, firstOwner.Username)
	secondSource := createTestAccount(t, secondOwner.Username)
	destination := createTestAccount(t, recipient.Username)
	key := uuid.New()

	first, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  firstSource.ID,
		ToAccountID:    destination.ID,
		Amount:         10,
		Currency:       currency.USD,
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  secondSource.ID,
		ToAccountID:    destination.ID,
		Amount:         20,
		Currency:       currency.USD,
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Transfer.ID == first.Transfer.ID || second.Transfer.FromAccountID != secondSource.ID {
		t.Fatalf("cross-source key replayed original transfer: %+v", second.Transfer)
	}
}

func TestTransferIdempotencyRollbackPreconditionDetectsDuplicateKeys(t *testing.T) {
	firstOwner := createTestUser(t)
	secondOwner := createTestUser(t)
	recipient := createTestUser(t)
	firstSource := createTestAccount(t, firstOwner.Username)
	secondSource := createTestAccount(t, secondOwner.Username)
	destination := createTestAccount(t, recipient.Username)
	key := uuid.New()

	if _, err := testStore.CreateTransfer(t.Context(), sqlcdb.CreateTransferParams{
		FromAccountID:  firstSource.ID,
		ToAccountID:    destination.ID,
		Amount:         10,
		IdempotencyKey: key,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.CreateTransfer(t.Context(), sqlcdb.CreateTransferParams{
		FromAccountID:  secondSource.ID,
		ToAccountID:    destination.ID,
		Amount:         20,
		IdempotencyKey: key,
	}); err != nil {
		t.Fatal(err)
	}

	var duplicateExists bool
	if err := testPool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1
			FROM transfers
			WHERE idempotency_key = $1
			GROUP BY idempotency_key
			HAVING COUNT(*) > 1
		)`, key).Scan(&duplicateExists); err != nil {
		t.Fatal(err)
	}
	if !duplicateExists {
		t.Fatal("rollback precondition failed to detect duplicate idempotency key rows")
	}
}

func TestTransferTxRejectsMismatchedReplay(t *testing.T) {
	owner := createTestUser(t)
	recipientA := createTestUser(t)
	recipientB := createTestUser(t)
	source := createTestAccount(t, owner.Username)
	destinationA := createTestAccount(t, recipientA.Username)
	destinationB := createTestAccount(t, recipientB.Username)
	key := uuid.New()

	_, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  source.ID,
		ToAccountID:    destinationA.ID,
		Amount:         10,
		Currency:       currency.USD,
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  source.ID,
		ToAccountID:    destinationB.ID,
		Amount:         10,
		Currency:       currency.USD,
		IdempotencyKey: key,
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("want ErrIdempotencyConflict, got %v", err)
	}
}
