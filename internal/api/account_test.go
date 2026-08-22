package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"uuid"

	"github.com/vancanhuit/simplebank/internal/config"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

func TestCreateAccountOK(t *testing.T) {
	t.Parallel()
	var gotArg sqlcdb.CreateAccountParams
	fake := fakeStore{
		createAccountTx: func(_ context.Context, arg sqlcdb.CreateAccountParams) (sqlcdb.Account, error) {
			gotArg = arg
			return sqlcdb.Account{ID: uuid.New(), Owner: arg.Owner, Currency: arg.Currency}, nil
		},
	}
	s := newTestServerWithStore(t, fake)
	s.config.AccountOpeningLimits = map[string]int64{"USD": 1000}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(`{"currency":"USD","balance":500}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if gotArg.Owner != "alice" || gotArg.Currency != "USD" || gotArg.Balance != 500 {
		t.Fatalf("CreateAccountTx params = %+v, want owner alice, currency USD, balance 500", gotArg)
	}
}

func TestCreateAccountUnsupportedCurrency(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		createAccountTx: func(context.Context, sqlcdb.CreateAccountParams) (sqlcdb.Account, error) {
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
	t.Parallel()
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
	t.Parallel()
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

func TestCreateAccountOpeningBalanceLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		balance    int64
		wantStatus int
		wantStore  bool
	}{
		{name: "below", balance: 99999, wantStatus: http.StatusCreated, wantStore: true},
		{name: "equal", balance: 100000, wantStatus: http.StatusCreated, wantStore: true},
		{name: "above", balance: 100001, wantStatus: http.StatusUnprocessableEntity, wantStore: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			fake := fakeStore{createAccountTx: func(_ context.Context, arg sqlcdb.CreateAccountParams) (sqlcdb.Account, error) {
				called = true
				return sqlcdb.Account{ID: uuid.New(), Owner: arg.Owner, Balance: arg.Balance, Currency: arg.Currency}, nil
			}}
			s := newTestServerWithConfig(t, fake, config.Config{
				JWTSecret:            testSecret,
				AccessTTL:            time.Minute,
				RefreshTTL:           time.Hour,
				AccountOpeningLimits: map[string]int64{"USD": 100000},
			})
			body := fmt.Sprintf(`{"currency":"USD","balance":%d}`, tt.balance)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", bearer(t, "alice"))
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus || called != tt.wantStore {
				t.Fatalf("status=%d called=%v, want status=%d called=%v", rec.Code, called, tt.wantStatus, tt.wantStore)
			}
		})
	}
}

func TestCreateAccountMissingCurrencyLimitPermitsZeroRejectsPositive(t *testing.T) {
	t.Parallel()

	fake := fakeStore{createAccountTx: func(_ context.Context, arg sqlcdb.CreateAccountParams) (sqlcdb.Account, error) {
		return sqlcdb.Account{ID: uuid.New(), Owner: arg.Owner, Balance: arg.Balance, Currency: arg.Currency}, nil
	}}
	s := newTestServerWithConfig(t, fake, config.Config{
		JWTSecret:            testSecret,
		AccessTTL:            time.Minute,
		RefreshTTL:           time.Hour,
		AccountOpeningLimits: map[string]int64{"USD": 100000},
	})

	// Zero opening balance should succeed for EUR (not configured)
	reqZero := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(`{"currency":"EUR","balance":0}`))
	reqZero.Header.Set("Content-Type", "application/json")
	reqZero.Header.Set("Authorization", bearer(t, "alice"))
	recZero := httptest.NewRecorder()
	s.Handler().ServeHTTP(recZero, reqZero)
	if recZero.Code != http.StatusCreated {
		t.Errorf("zero balance for missing currency: want 201, got %d (%s)", recZero.Code, recZero.Body.String())
	}

	// Positive opening balance should be rejected for EUR (not configured)
	reqPos := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(`{"currency":"EUR","balance":1}`))
	reqPos.Header.Set("Content-Type", "application/json")
	reqPos.Header.Set("Authorization", bearer(t, "alice"))
	recPos := httptest.NewRecorder()
	s.Handler().ServeHTTP(recPos, reqPos)
	if recPos.Code != http.StatusUnprocessableEntity {
		t.Errorf("positive balance for missing currency: want 422, got %d (%s)", recPos.Code, recPos.Body.String())
	}
}

func TestCreateAccountUnsafeOpeningBalance(t *testing.T) {
	t.Parallel()

	fake := fakeStore{createAccountTx: func(context.Context, sqlcdb.CreateAccountParams) (sqlcdb.Account, error) {
		t.Fatal("store must not be reached for an unsafe opening balance")
		return sqlcdb.Account{}, nil
	}}
	s := newTestServerWithStore(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts",
		strings.NewReader(`{"currency":"USD","balance":9007199254740992}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for unsafe opening balance, got %d (%s)", rec.Code, rec.Body.String())
	}
	want := `{"error":"opening balance exceeds the supported limit"}` + "\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("want exact JSON %q, got %q", want, got)
	}
}
