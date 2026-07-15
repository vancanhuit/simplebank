package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vancanhuit/simplebank/internal/config"
	"github.com/vancanhuit/simplebank/internal/token"
)

func newHealthServer(t *testing.T, readiness func(context.Context) error) *Server {
	t.Helper()
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{JWTSecret: testSecret, AccessTTL: time.Minute, RefreshTTL: time.Hour}
	s, err := NewServer(cfg, nil, maker, nil, readiness)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestReadyzReady(t *testing.T) {
	s := newHealthServer(t, func(context.Context) error { return nil })
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 when readiness passes, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestReadyzUnavailable(t *testing.T) {
	s := newHealthServer(t, func(context.Context) error { return errors.New("db down") })
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 when readiness fails, got %d (%s)", rec.Code, rec.Body.String())
	}
}
