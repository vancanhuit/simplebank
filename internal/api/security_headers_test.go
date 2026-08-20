package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

const (
	wantContentSecurityPolicy = "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; connect-src 'self'"
	wantHSTS                  = "max-age=31536000; includeSubdomains"
)

func TestSecurityHeadersOnAPIResponse(t *testing.T) {
	t.Parallel()

	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "https://simplebank.test/livez", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	assertSecurityHeaders(t, rec)
	assertHSTS(t, rec)
}

func TestSecurityHeadersOnSPAResponse(t *testing.T) {
	t.Parallel()

	s := newTestServer(t)
	s.RegisterSPA(spaTestFS())
	req := httptest.NewRequest(http.MethodGet, "https://simplebank.test/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	assertSecurityHeaders(t, rec)
	assertHSTS(t, rec)
}

func TestHSTSOnlyOnHTTPS(t *testing.T) {
	t.Parallel()

	s := newTestServer(t)

	tests := []struct {
		name      string
		targetURL string
		wantHSTS  bool
	}{
		{name: "http", targetURL: "http://simplebank.test/livez", wantHSTS: false},
		{name: "https", targetURL: "https://simplebank.test/livez", wantHSTS: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, tt.targetURL, nil)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)

			if tt.wantHSTS {
				assertHSTS(t, rec)
			} else {
				assertSecurityHeadersWithoutHSTS(t, rec)
			}
		})
	}
}

func TestHSTSBehindHTTPSProxy(t *testing.T) {
	t.Parallel()

	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	req.Header.Set(echo.HeaderXForwardedProto, "https")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	assertSecurityHeaders(t, rec)
	assertHSTS(t, rec)
}

func assertSecurityHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if got := rec.Header().Get(echo.HeaderContentSecurityPolicy); got != wantContentSecurityPolicy {
		t.Fatalf("want CSP %q, got %q", wantContentSecurityPolicy, got)
	}
	if got := rec.Header().Get(echo.HeaderReferrerPolicy); got != "no-referrer" {
		t.Fatalf("want Referrer-Policy no-referrer, got %q", got)
	}
	if got := rec.Header().Get(echo.HeaderXFrameOptions); got != "DENY" {
		t.Fatalf("want X-Frame-Options DENY, got %q", got)
	}
	if got := rec.Header().Get(echo.HeaderXContentTypeOptions); got != "nosniff" {
		t.Fatalf("want X-Content-Type-Options nosniff, got %q", got)
	}
}

func assertSecurityHeadersWithoutHSTS(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	assertSecurityHeaders(t, rec)
	if got := rec.Header().Get(echo.HeaderStrictTransportSecurity); got != "" {
		t.Fatalf("want no HSTS over HTTP, got %q", got)
	}
}

func assertHSTS(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if got := rec.Header().Get(echo.HeaderStrictTransportSecurity); got != wantHSTS {
		t.Fatalf("want HSTS %q, got %q", wantHSTS, got)
	}
}
