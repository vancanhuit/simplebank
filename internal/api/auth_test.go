package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/token"
)

// bearer returns an "Authorization: Bearer <token>" value for a token signed
// with the same secret the test server's auth middleware verifies against.
func bearer(t *testing.T, username string) string {
	t.Helper()
	return bearerWithID(t, username, uuid.New())
}

func bearerWithID(t *testing.T, username string, id uuid.UUID) string {
	t.Helper()
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := maker.CreateTokenWithID(id, username, roleDepositor, token.Access, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + tok
}

func legacyBearerWithID(t *testing.T, username string, id uuid.UUID) string {
	t.Helper()
	payload := token.NewPayloadWithID(id, username, roleDepositor, token.Access, time.Minute)
	payload.SessionBound = false
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + raw
}

func protectedAccountsRequest(t *testing.T, s *Server, auth string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func assertProtectedRouteSessionResult(
	t *testing.T,
	username string,
	sessionID uuid.UUID,
	validate func(context.Context, uuid.UUID, string, time.Time) error,
	wantStatus int,
) *httptest.ResponseRecorder {
	t.Helper()
	validateCalled := false
	fake := fakeStore{
		validateAccessSession: func(ctx context.Context, id uuid.UUID, gotUsername string, now time.Time) error {
			validateCalled = true
			if id != sessionID {
				t.Fatalf("session ID = %s, want %s", id, sessionID)
			}
			if gotUsername != username {
				t.Fatalf("username = %q, want %q", gotUsername, username)
			}
			if now.IsZero() {
				t.Fatal("ValidateAccessSession must receive current time")
			}
			return validate(ctx, id, gotUsername, now)
		},
		listAccounts: func(context.Context, sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error) {
			t.Fatal("handler must not run when session validation fails")
			return nil, nil
		},
	}
	rec := protectedAccountsRequest(t, newTestServerWithStore(t, fake), bearerWithID(t, username, sessionID))
	if !validateCalled {
		t.Fatal("protected routes must validate access sessions")
	}
	if rec.Code != wantStatus {
		t.Fatalf("want %d, got %d (%s)", wantStatus, rec.Code, rec.Body.String())
	}
	return rec
}

func TestProtectedRouteRequiresAuth(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+uuid.NewString(), nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without token, got %d", rec.Code)
	}
}

func TestProtectedRouteRejectsBadToken(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+uuid.NewString(), nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for malformed token, got %d", rec.Code)
	}
}

func TestProtectedRouteRejectsRefreshToken(t *testing.T) {
	t.Parallel()
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := maker.CreateToken("alice", roleDepositor, token.Refresh, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	fake := fakeStore{
		listAccounts: func(context.Context, sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error) {
			t.Fatal("refresh token must be rejected before handler execution")
			return nil, nil
		},
	}
	s := newTestServerWithStore(t, fake)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for refresh bearer, got %d", rec.Code)
	}
}

func TestProtectedRouteRejectsLegacyAccessTokenByDefault(t *testing.T) {
	t.Parallel()
	sessionID := uuid.New()
	fake := fakeStore{
		listAccounts: func(context.Context, sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error) {
			return []sqlcdb.Account{}, nil
		},
	}

	rec := protectedAccountsRequest(
		t,
		newTestServerWithStore(t, fake),
		legacyBearerWithID(t, "alice", sessionID),
	)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for legacy token by default, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestProtectedRouteAcceptsLegacyAccessTokenWhenCompatibilityEnabled(t *testing.T) {
	t.Parallel()
	sessionID := uuid.New()
	fake := fakeStore{
		validateAccessSession: func(context.Context, uuid.UUID, string, time.Time) error {
			t.Fatal("legacy compatibility must not require a session row")
			return nil
		},
		listAccounts: func(context.Context, sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error) {
			return []sqlcdb.Account{}, nil
		},
	}
	s := newTestServerWithConfig(t, fake, config.Config{
		JWTSecret:               testSecret,
		AccessTTL:               time.Minute,
		RefreshTTL:              time.Hour,
		SessionCookieSecure:     true,
		AllowLegacyAccessTokens: true,
	})

	rec := protectedAccountsRequest(t, s, legacyBearerWithID(t, "alice", sessionID))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for compatible legacy token, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestProtectedRouteCompatibilityStillValidatesSessionBoundToken(t *testing.T) {
	t.Parallel()
	sessionID := uuid.New()
	validated := false
	fake := fakeStore{
		validateAccessSession: func(context.Context, uuid.UUID, string, time.Time) error {
			validated = true
			return store.ErrInvalidSession
		},
		listAccounts: func(context.Context, sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error) {
			t.Fatal("invalid session-bound token must not reach handler")
			return nil, nil
		},
	}
	s := newTestServerWithConfig(t, fake, config.Config{
		JWTSecret:               testSecret,
		AccessTTL:               time.Minute,
		RefreshTTL:              time.Hour,
		SessionCookieSecure:     true,
		AllowLegacyAccessTokens: true,
	})

	rec := protectedAccountsRequest(t, s, bearerWithID(t, "alice", sessionID))

	if !validated {
		t.Fatal("session-bound token must validate its PostgreSQL session")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for invalid bound session, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestProtectedRouteRejectsMissingSession(t *testing.T) {
	t.Parallel()
	assertProtectedRouteSessionResult(
		t,
		"alice",
		uuid.New(),
		func(context.Context, uuid.UUID, string, time.Time) error {
			return store.ErrInvalidSession
		},
		http.StatusUnauthorized,
	)
}

func TestProtectedRouteRejectsBlockedSession(t *testing.T) {
	t.Parallel()
	assertProtectedRouteSessionResult(
		t,
		"alice",
		uuid.New(),
		func(context.Context, uuid.UUID, string, time.Time) error {
			return store.ErrInvalidSession
		},
		http.StatusUnauthorized,
	)
}

func TestProtectedRouteRejectsExpiredSession(t *testing.T) {
	t.Parallel()
	assertProtectedRouteSessionResult(
		t,
		"alice",
		uuid.New(),
		func(context.Context, uuid.UUID, string, time.Time) error {
			return store.ErrInvalidSession
		},
		http.StatusUnauthorized,
	)
}

func TestProtectedRouteRejectsSessionUsernameMismatch(t *testing.T) {
	t.Parallel()
	assertProtectedRouteSessionResult(
		t,
		"alice",
		uuid.New(),
		func(context.Context, uuid.UUID, string, time.Time) error {
			return store.ErrInvalidSession
		},
		http.StatusUnauthorized,
	)
}

func TestProtectedRoutePropagatesSessionStoreError(t *testing.T) {
	t.Parallel()
	rec := assertProtectedRouteSessionResult(
		t,
		"alice",
		uuid.New(),
		func(context.Context, uuid.UUID, string, time.Time) error {
			return errors.New("session store down")
		},
		http.StatusInternalServerError,
	)
	if !strings.Contains(rec.Body.String(), "internal server error") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestGetAccountForbiddenWhenNotOwner(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, _ uuid.UUID) (sqlcdb.Account, error) {
			return sqlcdb.Account{ID: id, Owner: "bob", Currency: "USD"}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+id.String(), nil)
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 for non-owner, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestGetAccountOKForOwner(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, _ uuid.UUID) (sqlcdb.Account, error) {
			return sqlcdb.Account{ID: id, Owner: "alice", Currency: "USD"}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+id.String(), nil)
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for owner, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateTransferForbiddenWhenNotFromOwner(t *testing.T) {
	t.Parallel()
	fromID := uuid.New()
	toID := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, id uuid.UUID) (sqlcdb.Account, error) {
			return sqlcdb.Account{ID: id, Owner: "bob", Currency: "USD"}, nil
		},
		transferTx: func(_ context.Context, _ store.TransferTxParams) (store.TransferTxResult, error) {
			t.Fatal("TransferTx must not run when the caller does not own the source account")
			return store.TransferTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"from_account_id":"` + fromID.String() + `","to_account_id":"` + toID.String() + `","amount":10,"currency":"USD","idempotency_key":"` + uuid.NewString() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 transferring from another user's account, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateTransferSameAccount(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	fake := fakeStore{
		getAccount: func(_ context.Context, _ uuid.UUID) (sqlcdb.Account, error) {
			t.Fatal("store must not be reached for a same-account transfer")
			return sqlcdb.Account{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"from_account_id":"` + id.String() + `","to_account_id":"` + id.String() + `","amount":10,"currency":"USD","idempotency_key":"` + uuid.NewString() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer(t, "alice"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for same-account transfer, got %d (%s)", rec.Code, rec.Body.String())
	}
}
