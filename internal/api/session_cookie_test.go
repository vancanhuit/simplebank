package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

func TestSetRefreshCookie(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	expiresAt := time.Now().Add(time.Hour)
	s.setRefreshCookie(ctx, "refresh-token", expiresAt)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("want 1 cookie, got %d", len(cookies))
	}
	got := cookies[0]
	if got.Name != refreshCookieName || got.Value != "refresh-token" || got.Path != "/api/v1" {
		t.Fatalf("unexpected cookie: %+v", got)
	}
	if !got.HttpOnly || !got.Secure || got.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe refresh cookie: %+v", got)
	}
	if !got.Expires.Equal(expiresAt.UTC().Truncate(time.Second)) {
		t.Fatalf("cookie expiry = %v, want %v", got.Expires, expiresAt)
	}
}

func TestClearRefreshCookie(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	s.clearRefreshCookie(ctx)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("want 1 cookie, got %d", len(cookies))
	}
	got := cookies[0]
	if got.Name != refreshCookieName || got.Value != "" || got.Path != "/api/v1" {
		t.Fatalf("unexpected cleared cookie: %+v", got)
	}
	if got.MaxAge != -1 || !got.HttpOnly || !got.Secure || got.SameSite != http.SameSiteStrictMode {
		t.Fatalf("clear cookie flags = %+v", got)
	}
}
