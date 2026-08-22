package api

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"uuid"

	"github.com/vancanhuit/simplebank/internal/config"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

func TestSecurityHeadersAndAPINoStore(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transfer-limits", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	for name, want := range map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Cache-Control":           "no-store",
	} {
		if got := rec.Header().Get(name); got != want && (name != "Content-Security-Policy" || !strings.Contains(got, want)) {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestSecurityHeadersSetHSTSForTLS(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("Strict-Transport-Security = %q", got)
	}
}

func TestUntrustedForwardedProtoCannotEnableHSTS(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("untrusted forwarded proto enabled HSTS: %q", got)
	}
}

func TestCookieActionsRejectCrossOrigin(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		blockSession: func(context.Context, uuid.UUID) (sqlcdb.Session, error) {
			t.Fatal("cross-origin request must not reach the store")
			return sqlcdb.Session{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/logout", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set(headerSecFetchSite, "cross-site")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 for cross-origin cookie action, got %d", rec.Code)
	}
}

func TestCookieActionsAcceptConfiguredOrigin(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		JWTSecret:           testSecret,
		AccessTTL:           time.Minute,
		RefreshTTL:          time.Hour,
		PublicBaseURL:       "https://bank.example.com",
		SessionCookieSecure: true,
	}
	s := newTestServerWithConfig(t, nil, cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/logout", nil)
	req.Header.Set("Origin", "https://bank.example.com")
	req.Header.Set(headerSecFetchSite, "same-origin")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204 for configured origin, got %d", rec.Code)
	}
}

func TestCookieActionsAcceptTrustedProxyOrigin(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		JWTSecret:           testSecret,
		AccessTTL:           time.Minute,
		RefreshTTL:          time.Hour,
		TrustedProxies:      []string{"10.0.0.0/8"},
		SessionCookieSecure: true,
	}
	s := newTestServerWithConfig(t, nil, cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/logout", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("Origin", "https://bank.example.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set(headerXForwardedHost, "bank.example.com")
	req.Header.Set(headerSecFetchSite, "same-origin")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204 through trusted TLS proxy, got %d", rec.Code)
	}
}
