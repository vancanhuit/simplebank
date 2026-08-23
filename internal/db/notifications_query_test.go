//go:build integration

package store

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"uuid"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

func TestNotificationsSchemaEnforcesTransferAccountUniqueness(t *testing.T) {
	sender := createTestUser(t)
	recipient := createTestUser(t)
	from := createTestAccount(t, sender.Username)
	to := createTestAccount(t, recipient.Username)
	transfer, err := testStore.CreateTransfer(t.Context(), sqlcdb.CreateTransferParams{
		FromAccountID: from.ID, ToAccountID: to.ID, Amount: 25, IdempotencyKey: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}

	const insert = `INSERT INTO notifications
		(owner, account_id, transfer_id, direction, amount, currency, balance)
		VALUES ($1, $2, $3, 'sent', 25, 'USD', 975)`
	if _, err := testPool.Exec(t.Context(), insert, sender.Username, from.ID, transfer.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), insert, sender.Username, from.ID, transfer.ID); err == nil {
		t.Fatal("duplicate transfer/account notification succeeded")
	}
}

func TestCreateNotificationDerivesTransferData(t *testing.T) {
	sender := createTestUser(t)
	recipient := createTestUser(t)
	from := createTestAccount(t, sender.Username)
	to := createTestAccount(t, recipient.Username)

	result, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID:  from.ID,
		ToAccountID:    to.ID,
		Amount:         25,
		Currency:       "USD",
		IdempotencyKey: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}

	notification, err := testStore.CreateNotification(t.Context(), sqlcdb.CreateNotificationParams{
		Direction:  "sent",
		TransferID: result.Transfer.ID,
		AccountID:  from.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	if notification.Owner != sender.Username {
		t.Errorf("owner = %q, want %q", notification.Owner, sender.Username)
	}
	if notification.AccountID != from.ID {
		t.Errorf("account_id = %s, want %s", notification.AccountID, from.ID)
	}
	if notification.TransferID != result.Transfer.ID {
		t.Errorf("transfer_id = %s, want %s", notification.TransferID, result.Transfer.ID)
	}
	if notification.Direction != "sent" {
		t.Errorf("direction = %q, want sent", notification.Direction)
	}
	if notification.Amount != 25 {
		t.Errorf("amount = %d, want 25", notification.Amount)
	}
	if notification.Currency != "USD" {
		t.Errorf("currency = %q, want USD", notification.Currency)
	}
	if notification.Balance != 975 {
		t.Errorf("balance = %d, want 975", notification.Balance)
	}
	if notification.ReadAt.Valid {
		t.Error("new notification is already read")
	}
}

func TestListNotificationsOwnerScopeAndStableCursor(t *testing.T) {
	owner := createTestUser(t)
	recipient := createTestUser(t)
	account := createTestAccount(t, owner.Username)
	recipientAccount := createTestAccount(t, recipient.Username)
	ctx := t.Context()

	create := func(accountID uuid.UUID, direction string) sqlcdb.Notification {
		t.Helper()
		transfer, err := testStore.CreateTransfer(ctx, sqlcdb.CreateTransferParams{
			FromAccountID:  account.ID,
			ToAccountID:    recipientAccount.ID,
			Amount:         25,
			IdempotencyKey: uuid.New(),
		})
		if err != nil {
			t.Fatal(err)
		}
		notification, err := testStore.CreateNotification(ctx, sqlcdb.CreateNotificationParams{
			Direction:  direction,
			TransferID: transfer.ID,
			AccountID:  accountID,
		})
		if err != nil {
			t.Fatal(err)
		}
		return notification
	}

	notifications := []sqlcdb.Notification{
		create(account.ID, "sent"),
		create(account.ID, "sent"),
		create(account.ID, "sent"),
	}
	other := create(recipientAccount.ID, "received")
	createdAt := time.Date(2026, time.August, 23, 9, 0, 0, 0, time.UTC)
	for _, notification := range append(notifications, other) {
		if _, err := testPool.Exec(ctx,
			"UPDATE notifications SET created_at = $1 WHERE id = $2",
			createdAt, notification.ID,
		); err != nil {
			t.Fatal(err)
		}
	}

	wantIDs := make([]uuid.UUID, len(notifications))
	for i, notification := range notifications {
		wantIDs[i] = notification.ID
	}
	slices.SortFunc(wantIDs, func(a, b uuid.UUID) int {
		return strings.Compare(b.String(), a.String())
	})

	firstPage, err := testStore.ListNotifications(ctx, sqlcdb.ListNotificationsParams{
		Owner:     owner.Username,
		PageLimit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPage) != 2 {
		t.Fatalf("first page len = %d, want 2", len(firstPage))
	}
	for i := range firstPage {
		if firstPage[i].ID != wantIDs[i] {
			t.Errorf("first page[%d] = %s, want %s", i, firstPage[i].ID, wantIDs[i])
		}
		if firstPage[i].Owner != owner.Username {
			t.Errorf("first page[%d] owner = %q, want %q", i, firstPage[i].Owner, owner.Username)
		}
	}

	secondPage, err := testStore.ListNotifications(ctx, sqlcdb.ListNotificationsParams{
		Owner:           owner.Username,
		HasCursor:       true,
		CursorCreatedAt: firstPage[1].CreatedAt,
		CursorID:        firstPage[1].ID,
		PageLimit:       2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPage) != 1 {
		t.Fatalf("second page len = %d, want 1", len(secondPage))
	}
	if secondPage[0].ID != wantIDs[2] {
		t.Errorf("second page[0] = %s, want %s", secondPage[0].ID, wantIDs[2])
	}
	if secondPage[0].ID == firstPage[0].ID || secondPage[0].ID == firstPage[1].ID {
		t.Errorf("second page duplicated first-page id %s", secondPage[0].ID)
	}
	if secondPage[0].ID == other.ID {
		t.Errorf("notification for owner %q leaked into %q history", other.Owner, owner.Username)
	}
}

func TestNotificationReadQueriesAreOwnerScopedAndIdempotent(t *testing.T) {
	sender := createTestUser(t)
	recipient := createTestUser(t)
	from := createTestAccount(t, sender.Username)
	to := createTestAccount(t, recipient.Username)
	ctx := t.Context()

	createTransfer := func() sqlcdb.Transfer {
		t.Helper()
		transfer, err := testStore.CreateTransfer(ctx, sqlcdb.CreateTransferParams{
			FromAccountID:  from.ID,
			ToAccountID:    to.ID,
			Amount:         25,
			IdempotencyKey: uuid.New(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return transfer
	}
	createNotification := func(transfer sqlcdb.Transfer, accountID uuid.UUID, direction string) sqlcdb.Notification {
		t.Helper()
		notification, err := testStore.CreateNotification(ctx, sqlcdb.CreateNotificationParams{
			Direction:  direction,
			TransferID: transfer.ID,
			AccountID:  accountID,
		})
		if err != nil {
			t.Fatal(err)
		}
		return notification
	}

	firstTransfer := createTransfer()
	firstSenderNotification := createNotification(firstTransfer, from.ID, "sent")
	createNotification(firstTransfer, to.ID, "received")
	secondSenderNotification := createNotification(createTransfer(), from.ID, "sent")

	_, err := testStore.MarkNotificationRead(ctx, sqlcdb.MarkNotificationReadParams{
		ID:    firstSenderNotification.ID,
		Owner: recipient.Username,
	})
	if !errors.Is(ClassifyError(err), ErrRecordNotFound) {
		t.Fatalf("wrong-owner MarkNotificationRead error = %v, want %v", err, ErrRecordNotFound)
	}

	read, err := testStore.MarkNotificationRead(ctx, sqlcdb.MarkNotificationReadParams{
		ID:    firstSenderNotification.ID,
		Owner: sender.Username,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !read.ReadAt.Valid {
		t.Fatal("read_at is null after MarkNotificationRead")
	}
	readAgain, err := testStore.MarkNotificationRead(ctx, sqlcdb.MarkNotificationReadParams{
		ID:    firstSenderNotification.ID,
		Owner: sender.Username,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !readAgain.ReadAt.Time.Equal(read.ReadAt.Time) {
		t.Errorf("second MarkNotificationRead changed read_at from %s to %s", read.ReadAt.Time, readAgain.ReadAt.Time)
	}

	rows, err := testStore.MarkAllNotificationsRead(ctx, recipient.Username)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("first MarkAllNotificationsRead rows = %d, want 1", rows)
	}
	rows, err = testStore.MarkAllNotificationsRead(ctx, recipient.Username)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("second MarkAllNotificationsRead rows = %d, want 0", rows)
	}

	senderUnread, err := testStore.CountUnreadNotifications(ctx, sender.Username)
	if err != nil {
		t.Fatal(err)
	}
	if senderUnread != 1 {
		t.Errorf("sender unread = %d, want 1 for notification %s", senderUnread, secondSenderNotification.ID)
	}
	recipientUnread, err := testStore.CountUnreadNotifications(ctx, recipient.Username)
	if err != nil {
		t.Fatal(err)
	}
	if recipientUnread != 0 {
		t.Errorf("recipient unread = %d, want 0", recipientUnread)
	}
}
