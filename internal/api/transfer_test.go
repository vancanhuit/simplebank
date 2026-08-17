package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

func postTransfer(t *testing.T, s *Server, from, to uuid.UUID, currency, username string) *httptest.ResponseRecorder {
	t.Helper()
	return postTransferWithKey(t, s, from, to, currency, username, uuid.NewString())
}

func postTransferWithKey(t *testing.T, s *Server, from, to uuid.UUID, currency, username, key string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"from_account_id":"` + from.String() + `","to_account_id":"` + to.String() +
		`","amount":10,"currency":"` + currency + `","idempotency_key":"` + key + `"}`
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
	var gotArg store.TransferTxParams
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
			gotArg = arg
			return store.TransferTxResult{
				Transfer: sqlcdb.Transfer{ID: uuid.New(), FromAccountID: arg.FromAccountID, ToAccountID: arg.ToAccountID, Amount: arg.Amount},
			}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	key := uuid.NewString()
	rec := postTransferWithKey(t, s, fromID, toID, "USD", "alice", key)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !transferred {
		t.Fatal("TransferTx should run for an authorized, valid transfer")
	}
	if gotArg.Currency != "USD" {
		t.Errorf("currency not forwarded to store: got %q", gotArg.Currency)
	}
	if gotArg.IdempotencyKey.String() != key {
		t.Errorf("idempotency key not forwarded: got %q, want %q", gotArg.IdempotencyKey, key)
	}
}

func TestCreateTransferMissingIdempotencyKey(t *testing.T) {
	t.Parallel()
	fromID := uuid.New()
	toID := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			return sqlcdb.Account{ID: id, Owner: "alice", Currency: "USD"}, nil
		},
		transferTx: func(context.Context, store.TransferTxParams) (store.TransferTxResult, error) {
			t.Fatal("TransferTx must not run without an idempotency key")
			return store.TransferTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"from_account_id":"` + fromID.String() + `","to_account_id":"` + toID.String() +
		`","amount":10,"currency":"USD"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing idempotency key, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateTransferExceedsMaxAmount(t *testing.T) {
	t.Parallel()
	fromID := uuid.New()
	toID := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			owner := "bob"
			if id == fromID {
				owner = "alice"
			}
			return sqlcdb.Account{ID: id, Owner: owner, Currency: "USD"}, nil
		},
		transferTx: func(context.Context, store.TransferTxParams) (store.TransferTxResult, error) {
			t.Fatal("TransferTx must not run when the amount exceeds the per-transfer cap")
			return store.TransferTxResult{}, nil
		},
	}
	// Cap of 5 minor units for USD; the helper posts an amount of 10.
	s := newTestServerWithConfig(t, fake, config.Config{
		JWTSecret:  testSecret,
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
		TransferLimits: map[string]config.CurrencyLimit{
			"USD": {MaxPerTransfer: 5},
		},
	})

	rec := postTransfer(t, s, fromID, toID, "USD", "alice")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for over-limit amount, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateTransferUnsafeAmount(t *testing.T) {
	t.Parallel()
	fromID := uuid.New()
	toID := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			owner := "bob"
			if id == fromID {
				owner = "alice"
			}
			return sqlcdb.Account{ID: id, Owner: owner, Currency: "USD"}, nil
		},
		transferTx: func(context.Context, store.TransferTxParams) (store.TransferTxResult, error) {
			t.Fatal("TransferTx must not run when the amount is unsafe for JavaScript")
			return store.TransferTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"from_account_id":"` + fromID.String() +
		`","to_account_id":"` + toID.String() +
		`","amount":9007199254740992,"currency":"USD","idempotency_key":"` +
		uuid.NewString() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for unsafe transfer amount, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "amount exceeds the supported limit") {
		t.Errorf("want 'amount exceeds the supported limit', got %s", rec.Body.String())
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

func TestCreateTransferAuthorizesSourceBeforeDestinationLookup(t *testing.T) {
	t.Parallel()
	fromID := uuid.New()
	toID := uuid.New()
	destinationLookedUp := false
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			if id == fromID {
				return sqlcdb.Account{ID: id, Owner: "bob", Currency: "USD"}, nil
			}
			destinationLookedUp = true
			return sqlcdb.Account{ID: id, Owner: "carol", Currency: "USD"}, nil
		},
		transferTx: func(context.Context, store.TransferTxParams) (store.TransferTxResult, error) {
			t.Fatal("TransferTx must not run for an unauthorized source account")
			return store.TransferTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := postTransfer(t, s, fromID, toID, "USD", "alice")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 for unauthorized source account, got %d (%s)", rec.Code, rec.Body.String())
	}
	if destinationLookedUp {
		t.Fatal("destination account must not be disclosed through lookup before source authorization")
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
