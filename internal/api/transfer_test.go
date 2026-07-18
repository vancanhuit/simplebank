package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

func postTransfer(t *testing.T, s *Server, from, to uuid.UUID, currency, username string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"from_account_id":"` + from.String() + `","to_account_id":"` + to.String() +
		`","amount":10,"currency":"` + currency + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer(t, username))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestCreateTransferOK(t *testing.T) {
	t.Parallel()
	fromID := uuid.New()
	toID := uuid.New()
	var transferred bool
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			owner := "bob"
			if id == fromID {
				owner = "alice"
			}
			return sqlcdb.Account{ID: id, Owner: owner, Currency: "USD"}, nil
		},
		transferTx: func(_ context.Context, arg store.TransferTxParams) (store.TransferTxResult, error) {
			transferred = true
			return store.TransferTxResult{
				Transfer: sqlcdb.Transfer{ID: uuid.New(), FromAccountID: arg.FromAccountID, ToAccountID: arg.ToAccountID, Amount: arg.Amount},
			}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := postTransfer(t, s, fromID, toID, "USD", "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !transferred {
		t.Fatal("TransferTx should run for an authorized, valid transfer")
	}
}

func TestCreateTransferCurrencyMismatch(t *testing.T) {
	t.Parallel()
	fromID := uuid.New()
	toID := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			// Source account is EUR while the request asks for a USD transfer.
			return sqlcdb.Account{ID: id, Owner: "alice", Currency: "EUR"}, nil
		},
		transferTx: func(context.Context, store.TransferTxParams) (store.TransferTxResult, error) {
			t.Fatal("TransferTx must not run on a currency mismatch")
			return store.TransferTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := postTransfer(t, s, fromID, toID, "USD", "alice")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for currency mismatch, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateTransferToAccountNotFound(t *testing.T) {
	t.Parallel()
	fromID := uuid.New()
	toID := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			if id == fromID {
				return sqlcdb.Account{ID: id, Owner: "alice", Currency: "USD"}, nil
			}
			return sqlcdb.Account{}, store.ErrRecordNotFound
		},
		transferTx: func(context.Context, store.TransferTxParams) (store.TransferTxResult, error) {
			t.Fatal("TransferTx must not run when the destination account is missing")
			return store.TransferTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := postTransfer(t, s, fromID, toID, "USD", "alice")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for missing destination account, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func getTransfers(t *testing.T, s *Server, account uuid.UUID, query, username string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+account.String()+"/transfers"+query, nil)
	req.Header.Set("Authorization", bearer(t, username))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestListTransfersOK(t *testing.T) {
	t.Parallel()
	accountID := uuid.New()
	var gotLimit, gotOffset int32
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			return sqlcdb.Account{ID: id, Owner: "alice", Currency: "USD"}, nil
		},
		listTransfers: func(_ context.Context, arg sqlcdb.ListTransfersByAccountParams) ([]sqlcdb.Transfer, error) {
			gotLimit = arg.PageLimit
			gotOffset = arg.PageOffset
			return []sqlcdb.Transfer{{ID: uuid.New(), FromAccountID: arg.AccountID, ToAccountID: uuid.New(), Amount: 100}}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := getTransfers(t, s, accountID, "", "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if gotLimit != 10 {
		t.Errorf("default size not applied: PageLimit = %d, want 10", gotLimit)
	}
	if gotOffset != 0 {
		t.Errorf("unexpected offset: PageOffset = %d, want 0", gotOffset)
	}
}

func TestListTransfersForbiddenForNonOwner(t *testing.T) {
	t.Parallel()
	accountID := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			return sqlcdb.Account{ID: id, Owner: "bob", Currency: "USD"}, nil
		},
		listTransfers: func(context.Context, sqlcdb.ListTransfersByAccountParams) ([]sqlcdb.Transfer, error) {
			t.Fatal("history must not be read for an account the caller does not own")
			return nil, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := getTransfers(t, s, accountID, "", "alice")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 for non-owner, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestListTransfersClampsOversizePage checks the page-size cap: size 500 must be
// clamped to 100 and page 0 normalized to 1 (offset 0).
func TestListTransfersClampsOversizePage(t *testing.T) {
	t.Parallel()
	accountID := uuid.New()
	var gotLimit, gotOffset int32
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			return sqlcdb.Account{ID: id, Owner: "alice", Currency: "USD"}, nil
		},
		listTransfers: func(_ context.Context, arg sqlcdb.ListTransfersByAccountParams) ([]sqlcdb.Transfer, error) {
			gotLimit = arg.PageLimit
			gotOffset = arg.PageOffset
			return []sqlcdb.Transfer{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := getTransfers(t, s, accountID, "?page=0&size=500", "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if gotLimit != 100 {
		t.Errorf("size not capped: PageLimit = %d, want 100", gotLimit)
	}
	if gotOffset != 0 {
		t.Errorf("page not normalized: PageOffset = %d, want 0", gotOffset)
	}
}
