package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/token"
)

// bearer returns an "Authorization: Bearer <token>" value for a token signed
// with the same secret the test server's auth middleware verifies against.
func bearer(t *testing.T, username string) string {
	t.Helper()
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := maker.CreateToken(username, roleDepositor, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + tok
}

func TestProtectedRouteRequiresAuth(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+uuid.NewString(), nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without token, got %d", rec.Code)
	}
}

func TestProtectedRouteRejectsBadToken(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+uuid.NewString(), nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for malformed token, got %d", rec.Code)
	}
}

func TestGetAccountForbiddenWhenNotOwner(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, _ uuid.UUID) (sqlcdb.Account, error) {
			return sqlcdb.Account{ID: id, Owner: "bob", Currency: "USD"}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+id.String(), nil)
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 for non-owner, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestGetAccountOKForOwner(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, _ uuid.UUID) (sqlcdb.Account, error) {
			return sqlcdb.Account{ID: id, Owner: "alice", Currency: "USD"}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+id.String(), nil)
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for owner, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateTransferForbiddenWhenNotFromOwner(t *testing.T) {
	t.Parallel()
	fromID := uuid.New()
	toID := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			return sqlcdb.Account{ID: id, Owner: "bob", Currency: "USD"}, nil
		},
		transferTx: func(_ context.Context, _ store.TransferTxParams) (store.TransferTxResult, error) {
			t.Fatal("TransferTx must not run when the caller does not own the source account")
			return store.TransferTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"from_account_id":"` + fromID.String() + `","to_account_id":"` + toID.String() + `","amount":10,"currency":"USD","idempotency_key":"` + uuid.NewString() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 transferring from another user's account, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateTransferSameAccount(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, _ uuid.UUID) (sqlcdb.Account, error) {
			t.Fatal("store must not be reached for a same-account transfer")
			return sqlcdb.Account{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"from_account_id":"` + id.String() + `","to_account_id":"` + id.String() + `","amount":10,"currency":"USD","idempotency_key":"` + uuid.NewString() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for same-account transfer, got %d (%s)", rec.Code, rec.Body.String())
	}
}
