package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vancanhuit/simplebank/internal/config"
	"github.com/vancanhuit/simplebank/internal/token"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	maker, err := token.NewJWTMaker("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(config.Config{JWTSecret: "01234567890123456789012345678901"}, nil, maker, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreateUserBadRequest(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"username":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}
