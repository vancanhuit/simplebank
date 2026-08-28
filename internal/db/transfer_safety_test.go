//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"uuid"

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
	var notificationCount int
	if err := testPool.QueryRow(t.Context(),
		`SELECT count(*) FROM notifications WHERE transfer_id = $1`, first.Transfer.ID,
	).Scan(&notificationCount); err != nil {
		t.Fatal(err)
	}
	if notificationCount != 2 {
		t.Fatalf("notifications = %d, want 2", notificationCount)
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
	listener, err := pgx.ConnectConfig(t.Context(), testPool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelCleanup()
		if _, unlistenErr := listener.Exec(cleanupCtx, `UNLISTEN balance_notifications`); unlistenErr != nil {
			t.Errorf("unlisten balance notifications: %v", unlistenErr)
		}
		if closeErr := listener.Close(cleanupCtx); closeErr != nil {
			t.Errorf("close notification listener: %v", closeErr)
		}
	}()
	if _, err := listener.Exec(t.Context(), `LISTEN balance_notifications`); err != nil {
		t.Fatal(err)
	}

	key := uuid.New()
	amount := int64(100)
	n := min(8, int(testPool.Config().MaxConns)-2)
	if n < 2 {
		t.Fatalf("database pool needs at least 4 connections, got %d", testPool.Config().MaxConns)
	}
	type outcome struct {
		result TransferTxResult
		err    error
	}
	outcomes := make(chan outcome, n)
	start := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	blockerPID, releaseAccountLock := holdAccountLock(t, firstLockedAccountID(acc1.ID, acc2.ID))
	for range n {
		go func() {
			<-start
			result, err := testStore.TransferTx(ctx, TransferTxParams{
				FromAccountID:  acc1.ID,
				ToAccountID:    acc2.ID,
				Amount:         amount,
				Currency:       currency.USD,
				IdempotencyKey: key,
			})
			outcomes <- outcome{result: result, err: err}
		}()
	}
	close(start)
	waitForBlockedWorkers(t, blockerPID, n)
	releaseAccountLock()

	var transferID uuid.UUID
	for range n {
		outcome := <-outcomes
		if outcome.err != nil {
			t.Fatalf("concurrent replay failed: %v", outcome.err)
		}
		if outcome.result.Transfer.ID == uuid.Nil() {
			t.Fatal("concurrent replay returned an empty transfer")
		}
		if transferID == uuid.Nil() {
			transferID = outcome.result.Transfer.ID
		} else if outcome.result.Transfer.ID != transferID {
			t.Fatalf("concurrent replay returned transfer %s, want %s", outcome.result.Transfer.ID, transferID)
		}
	}

	var transferCount, entryCount int
	if err := testPool.QueryRow(t.Context(),
		`SELECT count(*) FROM transfers WHERE from_account_id = $1 AND idempotency_key = $2`,
		acc1.ID, key,
	).Scan(&transferCount); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(),
		`SELECT count(*) FROM entries WHERE account_id = $1 OR account_id = $2`,
		acc1.ID, acc2.ID,
	).Scan(&entryCount); err != nil {
		t.Fatal(err)
	}
	if transferCount != 1 || entryCount != 2 {
		t.Fatalf("persisted rows = %d transfers and %d entries, want 1 and 2", transferCount, entryCount)
	}
	var notificationCount int
	if err := testPool.QueryRow(t.Context(),
		`SELECT count(*) FROM notifications WHERE transfer_id = $1`, transferID,
	).Scan(&notificationCount); err != nil {
		t.Fatal(err)
	}
	if notificationCount != 2 {
		t.Fatalf("notifications = %d, want 2", notificationCount)
	}
	wantOwners := map[string]bool{u1.Username: false, u2.Username: false}
	for range 2 {
		waitCtx, cancelWait := context.WithTimeout(t.Context(), 2*time.Second)
		message, waitErr := listener.WaitForNotification(waitCtx)
		cancelWait()
		if waitErr != nil {
			t.Fatal(waitErr)
		}
		var payload struct {
			Owner string `json:"owner"`
		}
		if err := json.Unmarshal([]byte(message.Payload), &payload); err != nil {
			t.Fatal(err)
		}
		if seen, ok := wantOwners[payload.Owner]; !ok || seen {
			t.Fatalf("unexpected or duplicate notification owner %q", payload.Owner)
		}
		wantOwners[payload.Owner] = true
	}
	extraCtx, cancelExtra := context.WithTimeout(t.Context(), 200*time.Millisecond)
	_, err = listener.WaitForNotification(extraCtx)
	cancelExtra()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("third concurrent same-key notification error = %v, want deadline exceeded", err)
	}

	updated1, err := testStore.GetAccount(t.Context(), acc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.Balance != 1000-amount {
		t.Errorf("balance moved more than once: %d, want %d", updated1.Balance, 1000-amount)
	}
	updated2, err := testStore.GetAccount(t.Context(), acc2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated2.Balance != 1000+amount {
		t.Errorf("destination balance = %d, want %d", updated2.Balance, 1000+amount)
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
	if testPool.Config().MaxConns < 4 {
		t.Fatalf("database pool needs at least 4 connections, got %d", testPool.Config().MaxConns)
	}
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	type outcome struct {
		result TransferTxResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	start := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	blockerPID, releaseAccountLock := holdAccountLock(t, firstLockedAccountID(acc1.ID, acc2.ID))
	for range 2 {
		go func() {
			<-start
			result, err := testStore.TransferTx(ctx, TransferTxParams{
				FromAccountID:  acc1.ID,
				ToAccountID:    acc2.ID,
				Amount:         60,
				Currency:       currency.USD,
				IdempotencyKey: uuid.New(),
				DailyLimit:     100,
			})
			outcomes <- outcome{result: result, err: err}
		}()
	}
	close(start)
	waitForBlockedWorkers(t, blockerPID, 2)
	releaseAccountLock()

	var succeeded, rejected int
	for range 2 {
		outcome := <-outcomes
		switch {
		case outcome.err == nil:
			succeeded++
			if outcome.result.Transfer.ID == uuid.Nil() {
				t.Fatal("successful transfer returned an empty result")
			}
		case errors.Is(outcome.err, ErrDailyLimitExceeded):
			rejected++
		default:
			t.Fatalf("unexpected concurrent transfer error: %v", outcome.err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent outcomes = %d succeeded and %d rejected, want 1 and 1", succeeded, rejected)
	}

	var transferCount, entryCount int
	if err := testPool.QueryRow(t.Context(),
		`SELECT count(*) FROM transfers WHERE from_account_id = $1`, acc1.ID,
	).Scan(&transferCount); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(),
		`SELECT count(*) FROM entries WHERE account_id = $1 OR account_id = $2`, acc1.ID, acc2.ID,
	).Scan(&entryCount); err != nil {
		t.Fatal(err)
	}
	if transferCount != 1 || entryCount != 2 {
		t.Fatalf("persisted rows = %d transfers and %d entries, want 1 and 2", transferCount, entryCount)
	}

	updated1, err := testStore.GetAccount(t.Context(), acc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated2, err := testStore.GetAccount(t.Context(), acc2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.Balance != 940 || updated2.Balance != 1060 {
		t.Fatalf("balances = %d and %d, want 940 and 1060", updated1.Balance, updated2.Balance)
	}
}

// TestTransferTxDailyLimitDoesNotOverflow ensures a large historical total
// cannot wrap the limit comparison and permit another transfer.
func TestTransferTxDailyLimitDoesNotOverflow(t *testing.T) {
	sender := createTestUser(t)
	recipient := createTestUser(t)
	from := createTestAccount(t, sender.Username)
	to := createTestAccount(t, recipient.Username)

	if _, err := testPool.Exec(t.Context(), `
		INSERT INTO transfers (from_account_id, to_account_id, amount, idempotency_key)
		SELECT $1, $2, $3, uuidv7()
		FROM generate_series(1, 1024)
	`, from.ID, to.ID, int64(currency.MaxSafeMinorUnits)); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
		INSERT INTO transfers (from_account_id, to_account_id, amount, idempotency_key)
		VALUES ($1, $2, 1018, $3)
	`, from.ID, to.ID, uuid.New()); err != nil {
		t.Fatal(err)
	}

	_, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  from.ID,
		ToAccountID:    to.ID,
		Amount:         10,
		Currency:       currency.USD,
		IdempotencyKey: uuid.New(),
		DailyLimit:     currency.MaxSafeMinorUnits,
	})
	if !errors.Is(err, ErrDailyLimitExceeded) {
		t.Fatalf("want ErrDailyLimitExceeded, got %v", err)
	}

	updatedFrom, err := testStore.GetAccount(t.Context(), from.ID)
	if err != nil {
		t.Fatal(err)
	}
	updatedTo, err := testStore.GetAccount(t.Context(), to.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedFrom.Balance != from.Balance || updatedTo.Balance != to.Balance {
		t.Fatalf("balances changed after rejected transfer: %d and %d", updatedFrom.Balance, updatedTo.Balance)
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
