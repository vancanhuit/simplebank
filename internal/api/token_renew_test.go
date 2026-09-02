package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/secret"
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
	tok, payload, err := maker.CreateToken(username, roleDepositor, token.Refresh, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	session := sqlcdb.Session{
		ID:           payload.ID,
		Username:     username,
		RefreshToken: secret.Digest(tok),
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens/renew", nil)
	if refresh != "" {
		req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: refresh, Path: "/api/v1"})
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestRenewTokenOK(t *testing.T) {
	t.Parallel()
	tok, session := refreshToken(t, "alice", nil)
	var rotated store.RotateSessionTxParams
	var replacementSession store.SessionReplacement
	fake := fakeStore{
		rotateSessionTx: func(_ context.Context, arg store.RotateSessionTxParams) (sqlcdb.Session, error) {
			rotated = arg
			newSession, err := arg.NewSession()
			if err != nil {
				return sqlcdb.Session{}, err
			}
			replacementSession = newSession
			return sqlcdb.Session{ID: newSession.ID, Username: arg.Username, RefreshToken: newSession.RefreshTokenHash, ExpiresAt: newSession.ExpiresAt}, nil
		},
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: "alice", Email: "alice@example.com", IsEmailVerified: true}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := postRenew(t, s, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rotated.ID != session.ID {
		t.Fatalf("RotateSessionTx ID = %s, want %s", rotated.ID, session.ID)
	}
	if rotated.Username != session.Username {
		t.Fatalf("RotateSessionTx username = %q, want %q", rotated.Username, session.Username)
	}
	if rotated.RefreshTokenHash != secret.Digest(tok) {
		t.Fatalf("RotateSessionTx hash = %q, want %q", rotated.RefreshTokenHash, secret.Digest(tok))
	}
	if replacementSession.ID != session.ID {
		t.Fatalf("renew replacement session ID = %s, want stable %s", replacementSession.ID, session.ID)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["access_token"] == "" {
		t.Fatalf("renew response missing access token: %+v", body)
	}
	if _, ok := body["user"]; !ok {
		t.Fatalf("renew response missing user: %+v", body)
	}
	if _, ok := body["refresh_token"]; ok {
		t.Fatalf("renew response exposed refresh token: %+v", body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("want 1 cookie, got %d", len(cookies))
	}
	if got := cookies[0]; got.Name != refreshCookieName || got.Value == tok || got.Value == "" {
		t.Fatalf("renew must replace refresh cookie, got %+v", got)
	}
	newRefreshPayload, err := s.tokenMaker.VerifyToken(cookies[0].Value, token.Refresh)
	if err != nil {
		t.Fatalf("verify rotated refresh cookie: %v", err)
	}
	if newRefreshPayload.ID != session.ID {
		t.Fatalf("rotated refresh token ID = %s, want stable %s", newRefreshPayload.ID, session.ID)
	}
}

func TestRenewTokenUnverifiedUser(t *testing.T) {
	t.Parallel()
	tok, _ := refreshToken(t, "alice", nil)
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: "alice", IsEmailVerified: false}, nil
		},
		rotateSessionTx: func(context.Context, store.RotateSessionTxParams) (sqlcdb.Session, error) {
			t.Fatal("unverified user session must not be rotated")
			return sqlcdb.Session{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := postRenew(t, s, tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := decodeErrorResponse(t, rec); got.Code != "email_verification_required" {
		t.Fatalf("code = %q, want email_verification_required", got.Code)
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].Name != refreshCookieName || cookies[0].MaxAge != -1 {
		t.Fatalf("unverified user refresh cookie must be cleared, got %+v", cookies)
	}
}

func TestRenewTokenBlockedSession(t *testing.T) {
	t.Parallel()
	tok, _ := refreshToken(t, "alice", nil)
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: "alice", IsEmailVerified: true}, nil
		},
		rotateSessionTx: func(context.Context, store.RotateSessionTxParams) (sqlcdb.Session, error) {
			return sqlcdb.Session{}, store.ErrInvalidSession
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := postRenew(t, s, tok)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for blocked session, got %d", rec.Code)
	}
	if got := decodeErrorResponse(t, rec); got.Code != "invalid_session" {
		t.Fatalf("code = %q, want invalid_session", got.Code)
	}
}

func TestRenewTokenUsernameMismatch(t *testing.T) {
	t.Parallel()
	tok, _ := refreshToken(t, "alice", nil)
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: "alice", IsEmailVerified: true}, nil
		},
		rotateSessionTx: func(context.Context, store.RotateSessionTxParams) (sqlcdb.Session, error) {
			return sqlcdb.Session{}, store.ErrInvalidSession
		},
	}
	s := newTestServerWithStore(t, fake)

	if rec := postRenew(t, s, tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for username mismatch, got %d", rec.Code)
	}
}

func TestRenewTokenRefreshMismatch(t *testing.T) {
	t.Parallel()
	tok, _ := refreshToken(t, "alice", nil)
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: "alice", IsEmailVerified: true}, nil
		},
		rotateSessionTx: func(context.Context, store.RotateSessionTxParams) (sqlcdb.Session, error) {
			return sqlcdb.Session{}, store.ErrInvalidSession
		},
	}
	s := newTestServerWithStore(t, fake)

	if rec := postRenew(t, s, tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for refresh token mismatch, got %d", rec.Code)
	}
}

func TestRenewTokenExpiredSession(t *testing.T) {
	t.Parallel()
	tok, _ := refreshToken(t, "alice", nil)
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: "alice", IsEmailVerified: true}, nil
		},
		rotateSessionTx: func(context.Context, store.RotateSessionTxParams) (sqlcdb.Session, error) {
			return sqlcdb.Session{}, store.ErrInvalidSession
		},
	}
	s := newTestServerWithStore(t, fake)

	if rec := postRenew(t, s, tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for expired session, got %d", rec.Code)
	}
}

func TestRenewTokenInvalidToken(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	if rec := postRenew(t, s, "not-a-valid-token"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for invalid refresh token, got %d", rec.Code)
	}
}

func TestRenewTokenMissingCookie(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	if rec := postRenew(t, s, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("want 204 for missing refresh cookie, got %d", rec.Code)
	}
}

func TestRenewTokenRejectsCrossOrigin(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens/renew", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 for cross-origin renew, got %d", rec.Code)
	}
	if got := decodeErrorResponse(t, rec); got.Code != "cross_origin_denied" {
		t.Fatalf("code = %q, want cross_origin_denied", got.Code)
	}
}

func TestRenewTokenReusedOldCookie(t *testing.T) {
	t.Parallel()
	tok, _ := refreshToken(t, "alice", nil)
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: "alice", IsEmailVerified: true}, nil
		},
		rotateSessionTx: func(context.Context, store.RotateSessionTxParams) (sqlcdb.Session, error) {
			return sqlcdb.Session{}, store.ErrInvalidSession
		},
	}
	s := newTestServerWithStore(t, fake)

	if rec := postRenew(t, s, tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for reused rotated refresh token, got %d", rec.Code)
	}
}
