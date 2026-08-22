package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestCredentialRateLimitIsPerClientAndEndpoint(t *testing.T) {
	s := newTestServer(t)

	request := func(path, remoteAddr string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}

	for range 5 {
		if code := request("/api/v1/users/login", "192.0.2.1:1234").Code; code != http.StatusBadRequest {
			t.Fatalf("login request: got status %d, want %d", code, http.StatusBadRequest)
		}
	}

	limited := request("/api/v1/users/login", "192.0.2.1:1234")
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("limited request: got status %d, want %d", limited.Code, http.StatusTooManyRequests)
	}
	if retryAfter, err := strconv.Atoi(limited.Header().Get("Retry-After")); err != nil || retryAfter < 1 {
		t.Errorf("Retry-After = %q, want a positive number of seconds", limited.Header().Get("Retry-After"))
	}
	if got := limited.Header().Get("X-RateLimit-Limit"); got != "5" {
		t.Errorf("X-RateLimit-Limit = %q, want 5", got)
	}
	var body map[string]string
	if err := json.NewDecoder(limited.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "rate limit exceeded" {
		t.Errorf("error = %q, want rate limit exceeded", body["error"])
	}

	if code := request("/api/v1/users", "192.0.2.1:1234").Code; code != http.StatusBadRequest {
		t.Errorf("different endpoint: got status %d, want %d", code, http.StatusBadRequest)
	}
	if code := request("/api/v1/users/login", "192.0.2.2:1234").Code; code != http.StatusBadRequest {
		t.Errorf("different client: got status %d, want %d", code, http.StatusBadRequest)
	}
}
