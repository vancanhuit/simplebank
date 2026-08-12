package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vancanhuit/simplebank/internal/config"
)

func TestTransferLimitsEndpoint(t *testing.T) {
	t.Parallel()
	s := newTestServerWithConfig(t, nil, config.Config{
		JWTSecret:  testSecret,
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
		TransferLimits: map[string]config.CurrencyLimit{
			"USD": {MaxPerTransfer: 100000, Daily: 500000},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transfer-limits", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var got map[string]config.CurrencyLimit
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["USD"].MaxPerTransfer != 100000 || got["USD"].Daily != 500000 {
		t.Errorf("USD limit = %+v, want {100000 500000}", got["USD"])
	}
}

func TestTransferLimitsEndpointEmpty(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transfer-limits", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	// No limits configured must serialize as an object, not null.
	if body := rec.Body.String(); body != "{}\n" && body != "{}" {
		t.Errorf("empty limits body = %q, want {}", body)
	}
}

func TestAccountOpeningLimitsEndpoint(t *testing.T) {
	t.Parallel()
	s := newTestServerWithConfig(t, nil, config.Config{
		JWTSecret:            testSecret,
		AccessTTL:            time.Minute,
		RefreshTTL:           time.Hour,
		AccountOpeningLimits: map[string]int64{"USD": 100000, "VND": 25000000},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account-opening-limits", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var got map[string]int64
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["USD"] != 100000 || got["VND"] != 25000000 {
		t.Errorf("opening limits = %+v, want {USD:100000 VND:25000000}", got)
	}
}

func TestAccountOpeningLimitsEndpointEmpty(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account-opening-limits", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	// No limits configured must serialize as an object, not null.
	if body := rec.Body.String(); body != "{}\n" && body != "{}" {
		t.Errorf("empty limits body = %q, want {}", body)
	}
}
