//go:build integration

package store

import (
	"testing"

	"uuid"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

// TestListTransfersByAccount verifies the history query returns transfers where
// the account is either sender or receiver, orders them newest first, excludes
// transfers between unrelated accounts, and honors limit/offset paging.
func TestListTransfersByAccount(t *testing.T) {
	u1 := createTestUser(t)
	u2 := createTestUser(t)
	acc1 := createTestAccount(t, u1.Username)
	acc2 := createTestAccount(t, u2.Username)

	ctx := t.Context()
	mk := func(from, to uuid.UUID, amount int64) sqlcdb.Transfer {
		t.Helper()
		tr, err := testStore.CreateTransfer(ctx, sqlcdb.CreateTransferParams{
			FromAccountID:  from,
			ToAccountID:    to,
			Amount:         amount,
			IdempotencyKey: uuid.New(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return tr
	}

	// Two outgoing and one incoming for acc1, created in this order. uuidv7 ids
	// increase monotonically, so newest-first ordering is t3, t2, t1.
	t1 := mk(acc1.ID, acc2.ID, 100)
	t2 := mk(acc1.ID, acc2.ID, 200)
	t3 := mk(acc2.ID, acc1.ID, 50)

	// A transfer between unrelated accounts must not appear in acc1's history.
	u3 := createTestUser(t)
	u4 := createTestUser(t)
	acc3 := createTestAccount(t, u3.Username)
	acc4 := createTestAccount(t, u4.Username)
	other := mk(acc3.ID, acc4.ID, 999)

	got, err := testStore.ListTransfersByAccount(ctx, sqlcdb.ListTransfersByAccountParams{
		AccountID:  acc1.ID,
		PageLimit:  10,
		PageOffset: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	wantOrder := []uuid.UUID{t3.ID, t2.ID, t1.ID}
	if len(got) != len(wantOrder) {
		t.Fatalf("len = %d, want %d", len(got), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got[i].ID != want {
			t.Errorf("got[%d].ID = %s, want %s", i, got[i].ID, want)
		}
		if got[i].ID == other.ID {
			t.Errorf("unrelated transfer %s leaked into acc1 history", other.ID)
		}
	}

	// Paging: a page of 2 returns the two newest; the next page returns the rest.
	page1, err := testStore.ListTransfersByAccount(ctx, sqlcdb.ListTransfersByAccountParams{
		AccountID:  acc1.ID,
		PageLimit:  2,
		PageOffset: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 || page1[0].ID != t3.ID || page1[1].ID != t2.ID {
		t.Errorf("page1 = %v, want [%s %s]", ids(page1), t3.ID, t2.ID)
	}

	page2, err := testStore.ListTransfersByAccount(ctx, sqlcdb.ListTransfersByAccountParams{
		AccountID:  acc1.ID,
		PageLimit:  2,
		PageOffset: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 || page2[0].ID != t1.ID {
		t.Errorf("page2 = %v, want [%s]", ids(page2), t1.ID)
	}
}

func ids(transfers []sqlcdb.Transfer) []uuid.UUID {
	out := make([]uuid.UUID, len(transfers))
	for i, tr := range transfers {
		out[i] = tr.ID
	}
	return out
}
