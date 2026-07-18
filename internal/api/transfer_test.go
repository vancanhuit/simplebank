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
