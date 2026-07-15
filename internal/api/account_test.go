package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

func TestCreateAccountOK(t *testing.T) {
	fake := fakeStore{
		createAccount: func(_ context.Context, arg sqlcdb.CreateAccountParams) (sqlcdb.Account, error) {
			return sqlcdb.Account{ID: uuid.New(), Owner: arg.Owner, Currency: arg.Currency}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(`{"currency":"USD"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateAccountUnsupportedCurrency(t *testing.T) {
	fake := fakeStore{
		createAccount: func(context.Context, sqlcdb.CreateAccountParams) (sqlcdb.Account, error) {
			t.Fatal("store must not be reached for an unsupported currency")
			return sqlcdb.Account{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(`{"currency":"XYZ"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for unsupported currency, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestListAccountsClampsOversizePage checks the page-size cap: size 500 must be
// clamped to 100 and page 0 normalized to 1 (offset 0).
func TestListAccountsClampsOversizePage(t *testing.T) {
	var gotLimit, gotOffset int32
	fake := fakeStore{
		listAccounts: func(_ context.Context, arg sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error) {
			gotLimit = arg.Limit
			gotOffset = arg.Offset
			return []sqlcdb.Account{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts?page=0&size=500", nil)
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if gotLimit != 100 {
		t.Errorf("size not capped: Limit = %d, want 100", gotLimit)
	}
	if gotOffset != 0 {
		t.Errorf("page not normalized: Offset = %d, want 0", gotOffset)
	}
}

// TestListAccountsDefaultsSize checks size 0 falls back to the default of 5.
func TestListAccountsDefaultsSize(t *testing.T) {
	var gotLimit int32
	fake := fakeStore{
		listAccounts: func(_ context.Context, arg sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error) {
			gotLimit = arg.Limit
			return []sqlcdb.Account{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts?page=2&size=0", nil)
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if gotLimit != 5 {
		t.Errorf("size not defaulted: Limit = %d, want 5", gotLimit)
	}
}
