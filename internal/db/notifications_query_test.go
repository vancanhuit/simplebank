//go:build integration

package store

import (
	"context"
	"encoding/json"
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

	transfer, err := testStore.CreateTransfer(t.Context(), sqlcdb.CreateTransferParams{
		FromAccountID:  from.ID,
		ToAccountID:    to.ID,
		Amount:         25,
		IdempotencyKey: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	updatedFrom, err := testStore.AddAccountBalance(t.Context(), sqlcdb.AddAccountBalanceParams{
		ID: from.ID, Amount: -25,
	})
	if err != nil {
		t.Fatal(err)
	}

	notification, err := testStore.CreateNotification(t.Context(), sqlcdb.CreateNotificationParams{
		Direction:  "sent",
		TransferID: transfer.ID,
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
	if notification.TransferID != transfer.ID {
		t.Errorf("transfer_id = %s, want %s", notification.TransferID, transfer.ID)
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
	if notification.Balance != updatedFrom.Balance {
		t.Errorf("balance = %d, want %d", notification.Balance, updatedFrom.Balance)
	}
	if notification.ReadAt.Valid {
		t.Error("new notification is already read")
	}
}

func TestNotificationPublishIsCommitGated(t *testing.T) {
	listener, err := testPool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelCleanup()
		if _, unlistenErr := listener.Exec(cleanupCtx, `UNLISTEN balance_notifications`); unlistenErr != nil {
			t.Errorf("unlisten balance notifications: %v", unlistenErr)
		}
		listener.Release()
	}()
	if _, err := listener.Exec(t.Context(), `LISTEN balance_notifications`); err != nil {
		t.Fatal(err)
	}

	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	queries := sqlcdb.New(tx)
	if err := queries.PublishNotification(t.Context(), sqlcdb.PublishNotificationParams{
		NotificationID: uuid.New(),
		Owner:          "rolled-back-owner",
	}); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	rollbackCtx, cancelRollback := context.WithTimeout(t.Context(), 200*time.Millisecond)
	_, err = listener.Conn().WaitForNotification(rollbackCtx)
	cancelRollback()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("notification after rollback error = %v, want deadline exceeded", err)
	}

	sender := createTestUser(t)
	recipient := createTestUser(t)
	from := createTestAccount(t, sender.Username)
	to := createTestAccount(t, recipient.Username)
	arg := TransferTxParams{
		FromAccountID:  from.ID,
		ToAccountID:    to.ID,
		Amount:         25,
		Currency:       "USD",
		IdempotencyKey: uuid.New(),
	}
	result, err := testStore.TransferTx(t.Context(), arg)
	if err != nil {
		t.Fatal(err)
	}
	senderRows, err := testStore.ListNotifications(t.Context(), sqlcdb.ListNotificationsParams{
		Owner: sender.Username, PageLimit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	recipientRows, err := testStore.ListNotifications(t.Context(), sqlcdb.ListNotificationsParams{
		Owner: recipient.Username, PageLimit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(senderRows) != 1 || len(recipientRows) != 1 ||
		senderRows[0].TransferID != result.Transfer.ID || recipientRows[0].TransferID != result.Transfer.ID {
		t.Fatalf("durable transfer notifications = %v and %v, want one per owner", senderRows, recipientRows)
	}

	type notificationPayload struct {
		ID    uuid.UUID `json:"id"`
		Owner string    `json:"owner"`
	}
	wantOwners := map[string]bool{sender.Username: false, recipient.Username: false}
	wantIDs := map[uuid.UUID]bool{senderRows[0].ID: false, recipientRows[0].ID: false}
	for range 2 {
		waitCtx, cancelWait := context.WithTimeout(t.Context(), 2*time.Second)
		notification, err := listener.Conn().WaitForNotification(waitCtx)
		cancelWait()
		if err != nil {
			t.Fatal(err)
		}

		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(notification.Payload), &fields); err != nil {
			t.Fatalf("decode payload %q: %v", notification.Payload, err)
		}
		if len(fields) != 2 || fields["id"] == nil || fields["owner"] == nil {
			t.Fatalf("payload fields = %v, want exactly id and owner", fields)
		}
		var payload notificationPayload
		if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
			t.Fatalf("decode payload %q: %v", notification.Payload, err)
		}
		if _, ok := wantIDs[payload.ID]; !ok || wantIDs[payload.ID] {
			t.Fatalf("unexpected or duplicate notification id %s", payload.ID)
		}
		wantIDs[payload.ID] = true
		if _, ok := wantOwners[payload.Owner]; !ok || wantOwners[payload.Owner] {
			t.Fatalf("unexpected or duplicate notification owner %q", payload.Owner)
		}
		wantOwners[payload.Owner] = true
	}

	if _, err := testStore.TransferTx(t.Context(), arg); err != nil {
		t.Fatal(err)
	}
	replayCtx, cancelReplay := context.WithTimeout(t.Context(), 200*time.Millisecond)
	_, err = listener.Conn().WaitForNotification(replayCtx)
	cancelReplay()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("notification after idempotent replay error = %v, want deadline exceeded", err)
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

func TestListNotificationsPageReturnsSnapshotAndHasMore(t *testing.T) {
	owner := createTestUser(t)
	otherOwner := createTestUser(t)
	account := createTestAccount(t, owner.Username)
	otherAccount := createTestAccount(t, otherOwner.Username)
	ctx := t.Context()

	for range 3 {
		transfer, err := testStore.CreateTransfer(ctx, sqlcdb.CreateTransferParams{
			FromAccountID:  account.ID,
			ToAccountID:    otherAccount.ID,
			Amount:         25,
			IdempotencyKey: uuid.New(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testStore.CreateNotification(ctx, sqlcdb.CreateNotificationParams{
			Direction:  "sent",
			TransferID: transfer.ID,
			AccountID:  account.ID,
		}); err != nil {
			t.Fatal(err)
		}
	}

	page, err := testStore.ListNotificationsPage(ctx, ListNotificationsPageParams{
		Owner: owner.Username,
		Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Notifications) != 2 {
		t.Errorf("notifications len = %d, want 2", len(page.Notifications))
	}
	if !page.HasMore {
		t.Error("has_more = false, want true")
	}
	if page.UnreadCount != 3 {
		t.Errorf("unread_count = %d, want 3", page.UnreadCount)
	}
	for _, notification := range page.Notifications {
		if notification.Owner != owner.Username {
			t.Errorf("notification owner = %q, want %q", notification.Owner, owner.Username)
		}
	}
}

func TestListNotificationsPageUsesOneRepeatableReadSnapshot(t *testing.T) {
	owner := createTestUser(t)
	otherOwner := createTestUser(t)
	account := createTestAccount(t, owner.Username)
	otherAccount := createTestAccount(t, otherOwner.Username)
	ctx := t.Context()

	createNotification := func() {
		transfer, err := testStore.CreateTransfer(ctx, sqlcdb.CreateTransferParams{
			FromAccountID: account.ID, ToAccountID: otherAccount.ID, Amount: 25, IdempotencyKey: uuid.New(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testStore.CreateNotification(ctx, sqlcdb.CreateNotificationParams{
			Direction: "sent", TransferID: transfer.ID, AccountID: account.ID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	createNotification()

	snapshotStore := &SQLStore{Queries: sqlcdb.New(testPool), connPool: testPool}
	snapshotStore.afterListNotifications = createNotification
	page, err := snapshotStore.ListNotificationsPage(ctx, ListNotificationsPageParams{
		Owner: owner.Username,
		Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Notifications) != 1 || page.UnreadCount != 1 {
		t.Fatalf("snapshot page has %d rows and unread count %d, want 1 and 1", len(page.Notifications), page.UnreadCount)
	}

	unreadAfterCommit, err := testStore.CountUnreadNotifications(ctx, owner.Username)
	if err != nil {
		t.Fatal(err)
	}
	if unreadAfterCommit != 2 {
		t.Fatalf("unread count after concurrent commit = %d, want 2", unreadAfterCommit)
	}
}

func TestMarkNotificationReadTxReturnsAuthoritativeCount(t *testing.T) {
	owner := createTestUser(t)
	otherOwner := createTestUser(t)
	account := createTestAccount(t, owner.Username)
	otherAccount := createTestAccount(t, otherOwner.Username)
	ctx := t.Context()

	create := func() sqlcdb.Notification {
		t.Helper()
		transfer, err := testStore.CreateTransfer(ctx, sqlcdb.CreateTransferParams{
			FromAccountID:  account.ID,
			ToAccountID:    otherAccount.ID,
			Amount:         25,
			IdempotencyKey: uuid.New(),
		})
		if err != nil {
			t.Fatal(err)
		}
		notification, err := testStore.CreateNotification(ctx, sqlcdb.CreateNotificationParams{
			Direction:  "sent",
			TransferID: transfer.ID,
			AccountID:  account.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		return notification
	}

	first := create()
	create()

	unreadCount, err := testStore.MarkNotificationReadTx(ctx, owner.Username, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unreadCount != 1 {
		t.Errorf("first unread count = %d, want 1", unreadCount)
	}

	unreadCount, err = testStore.MarkNotificationReadTx(ctx, owner.Username, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unreadCount != 1 {
		t.Errorf("repeated unread count = %d, want 1", unreadCount)
	}

	_, err = testStore.MarkNotificationReadTx(ctx, otherOwner.Username, first.ID)
	if !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("wrong-owner error = %v, want %v", err, ErrRecordNotFound)
	}
}

func TestMarkAllNotificationsReadTxReturnsAuthoritativeCount(t *testing.T) {
	owner := createTestUser(t)
	otherOwner := createTestUser(t)
	account := createTestAccount(t, owner.Username)
	otherAccount := createTestAccount(t, otherOwner.Username)
	ctx := t.Context()

	for _, notificationAccount := range []struct {
		accountID uuid.UUID
		direction string
	}{
		{accountID: account.ID, direction: "sent"},
		{accountID: account.ID, direction: "sent"},
		{accountID: otherAccount.ID, direction: "received"},
	} {
		transfer, err := testStore.CreateTransfer(ctx, sqlcdb.CreateTransferParams{
			FromAccountID:  account.ID,
			ToAccountID:    otherAccount.ID,
			Amount:         25,
			IdempotencyKey: uuid.New(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testStore.CreateNotification(ctx, sqlcdb.CreateNotificationParams{
			Direction:  notificationAccount.direction,
			TransferID: transfer.ID,
			AccountID:  notificationAccount.accountID,
		}); err != nil {
			t.Fatal(err)
		}
	}

	unreadCount, err := testStore.MarkAllNotificationsReadTx(ctx, owner.Username)
	if err != nil {
		t.Fatal(err)
	}
	if unreadCount != 0 {
		t.Errorf("owner unread count = %d, want 0", unreadCount)
	}

	otherUnreadCount, err := testStore.CountUnreadNotifications(ctx, otherOwner.Username)
	if err != nil {
		t.Fatal(err)
	}
	if otherUnreadCount != 1 {
		t.Errorf("other owner unread count = %d, want 1", otherUnreadCount)
	}
}
