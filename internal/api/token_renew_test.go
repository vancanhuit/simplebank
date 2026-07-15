package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/token"
)

// refreshToken mints a refresh token plus a matching session row for renew
// tests. mutate lets each case corrupt one field to exercise a single guard.
func refreshToken(t *testing.T, username string, mutate func(*sqlcdb.Session)) (string, sqlcdb.Session) {
	t.Helper()
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	tok, payload, err := maker.CreateToken(username, roleDepositor, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	session := sqlcdb.Session{
		ID:           payload.ID,
		Username:     username,
		RefreshToken: tok,
		IsBlocked:    false,
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	if mutate != nil {
		mutate(&session)
	}
	return tok, session
}

func postRenew(t *testing.T, s *Server, refresh string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"refresh_token":"` + refresh + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens/renew", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestRenewTokenOK(t *testing.T) {
	tok, session := refreshToken(t, "alice", nil)
	fake := fakeStore{
		getSession: func(context.Context, uuid.UUID) (sqlcdb.Session, error) { return session, nil },
	}
	s := newTestServerWithStore(t, fake)

	rec := postRenew(t, s, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestRenewTokenBlockedSession(t *testing.T) {
	tok, session := refreshToken(t, "alice", func(s *sqlcdb.Session) { s.IsBlocked = true })
	fake := fakeStore{
		getSession: func(context.Context, uuid.UUID) (sqlcdb.Session, error) { return session, nil },
	}
	s := newTestServerWithStore(t, fake)

	if rec := postRenew(t, s, tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for blocked session, got %d", rec.Code)
	}
}

func TestRenewTokenUsernameMismatch(t *testing.T) {
	tok, session := refreshToken(t, "alice", func(s *sqlcdb.Session) { s.Username = "bob" })
	fake := fakeStore{
		getSession: func(context.Context, uuid.UUID) (sqlcdb.Session, error) { return session, nil },
	}
	s := newTestServerWithStore(t, fake)

	if rec := postRenew(t, s, tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for username mismatch, got %d", rec.Code)
	}
}

func TestRenewTokenRefreshMismatch(t *testing.T) {
	tok, session := refreshToken(t, "alice", func(s *sqlcdb.Session) { s.RefreshToken = "a-different-token" })
	fake := fakeStore{
		getSession: func(context.Context, uuid.UUID) (sqlcdb.Session, error) { return session, nil },
	}
	s := newTestServerWithStore(t, fake)

	if rec := postRenew(t, s, tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for refresh token mismatch, got %d", rec.Code)
	}
}

func TestRenewTokenExpiredSession(t *testing.T) {
	tok, session := refreshToken(t, "alice", func(s *sqlcdb.Session) { s.ExpiresAt = time.Now().Add(-time.Minute) })
	fake := fakeStore{
		getSession: func(context.Context, uuid.UUID) (sqlcdb.Session, error) { return session, nil },
	}
	s := newTestServerWithStore(t, fake)

	if rec := postRenew(t, s, tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for expired session, got %d", rec.Code)
	}
}

func TestRenewTokenInvalidToken(t *testing.T) {
	s := newTestServer(t)
	if rec := postRenew(t, s, "not-a-valid-token"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for invalid refresh token, got %d", rec.Code)
	}
}
