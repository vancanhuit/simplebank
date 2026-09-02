package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"uuid"

	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/password"
	"github.com/vancanhuit/simplebank/internal/token"
)

const (
	testSecret       = "01234567890123456789012345678901"
	testUserPassword = "correct horse battery staple"
)

// fakeStore satisfies store.Store by embedding the interface (nil underlying).
// Only the methods a test actually exercises are overridden; calling any other
// method panics, which is the desired signal for an unexpected store access.
type fakeStore struct {
	store.Store
	createUserTx               func(context.Context, store.CreateUserTxParams) (sqlcdb.User, error)
	getAccount                 func(context.Context, uuid.UUID) (sqlcdb.Account, error)
	transferTx                 func(context.Context, store.TransferTxParams) (store.TransferTxResult, error)
	getUser                    func(context.Context, string) (sqlcdb.User, error)
	getUserByEmail             func(context.Context, string) (sqlcdb.User, error)
	createSession              func(context.Context, sqlcdb.CreateSessionParams) (sqlcdb.Session, error)
	getSession                 func(context.Context, uuid.UUID) (sqlcdb.Session, error)
	rotateSessionTx            func(context.Context, store.RotateSessionTxParams) (sqlcdb.Session, error)
	blockSession               func(context.Context, uuid.UUID) (sqlcdb.Session, error)
	verifyEmailTx              func(context.Context, store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error)
	listAccounts               func(context.Context, sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error)
	createAccountTx            func(context.Context, sqlcdb.CreateAccountParams) (sqlcdb.Account, error)
	listTransfers              func(context.Context, sqlcdb.ListTransfersByAccountParams) ([]sqlcdb.Transfer, error)
	listNotificationsPage      func(context.Context, store.ListNotificationsPageParams) (store.ListNotificationsPageResult, error)
	markNotificationReadTx     func(context.Context, string, uuid.UUID) (int64, error)
	markAllNotificationsReadTx func(context.Context, string) (int64, error)
}

func (f fakeStore) CreateUserTx(ctx context.Context, arg store.CreateUserTxParams) (sqlcdb.User, error) {
	return f.createUserTx(ctx, arg)
}

func (f fakeStore) GetAccount(ctx context.Context, id uuid.UUID) (sqlcdb.Account, error) {
	return f.getAccount(ctx, id)
}

func (f fakeStore) TransferTx(ctx context.Context, arg store.TransferTxParams) (store.TransferTxResult, error) {
	return f.transferTx(ctx, arg)
}

func (f fakeStore) GetUser(ctx context.Context, username string) (sqlcdb.User, error) {
	return f.getUser(ctx, username)
}

func (f fakeStore) GetUserByEmail(ctx context.Context, email string) (sqlcdb.User, error) {
	return f.getUserByEmail(ctx, email)
}

func (f fakeStore) CreateSession(ctx context.Context, arg sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
	return f.createSession(ctx, arg)
}

func (f fakeStore) GetSession(ctx context.Context, id uuid.UUID) (sqlcdb.Session, error) {
	return f.getSession(ctx, id)
}

func (f fakeStore) RotateSessionTx(ctx context.Context, arg store.RotateSessionTxParams) (sqlcdb.Session, error) {
	return f.rotateSessionTx(ctx, arg)
}

func (f fakeStore) BlockSession(ctx context.Context, id uuid.UUID) (sqlcdb.Session, error) {
	return f.blockSession(ctx, id)
}

func (f fakeStore) VerifyEmailTx(ctx context.Context, arg store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error) {
	return f.verifyEmailTx(ctx, arg)
}

func (f fakeStore) ListAccounts(ctx context.Context, arg sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error) {
	return f.listAccounts(ctx, arg)
}

func (f fakeStore) CreateAccountTx(ctx context.Context, arg sqlcdb.CreateAccountParams) (sqlcdb.Account, error) {
	return f.createAccountTx(ctx, arg)
}

func (f fakeStore) ListTransfersByAccount(ctx context.Context, arg sqlcdb.ListTransfersByAccountParams) ([]sqlcdb.Transfer, error) {
	return f.listTransfers(ctx, arg)
}

func (f fakeStore) ListNotificationsPage(ctx context.Context, arg store.ListNotificationsPageParams) (store.ListNotificationsPageResult, error) {
	return f.listNotificationsPage(ctx, arg)
}

func (f fakeStore) MarkNotificationReadTx(ctx context.Context, owner string, id uuid.UUID) (int64, error) {
	return f.markNotificationReadTx(ctx, owner, id)
}

func (f fakeStore) MarkAllNotificationsReadTx(ctx context.Context, owner string) (int64, error) {
	return f.markAllNotificationsReadTx(ctx, owner)
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return newTestServerWithStore(t, nil)
}

func newTestServerWithStore(t *testing.T, st store.Store) *Server {
	t.Helper()
	return newTestServerWithConfig(t, st, config.Config{
		JWTSecret:           testSecret,
		AccessTTL:           time.Minute,
		RefreshTTL:          time.Hour,
		SessionCookieSecure: true,
	})
}

func newTestServerWithConfig(t *testing.T, st store.Store, cfg config.Config) *Server {
	t.Helper()
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(cfg, st, maker, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func decodeErrorResponse(t *testing.T, rec *httptest.ResponseRecorder) errorResponse {
	t.Helper()
	var got errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return got
}

func TestCreateUserBadRequest(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"username":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	if got := decodeErrorResponse(t, rec); got.Code != "invalid_request_payload" {
		t.Fatalf("code = %q, want invalid_request_payload", got.Code)
	}
}

func TestCreateUserMalformedBody(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"username":`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	if got := decodeErrorResponse(t, rec); got.Code != "invalid_request_body" {
		t.Fatalf("code = %q, want invalid_request_body", got.Code)
	}
}

func TestCreateUserOK(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		createUserTx: func(_ context.Context, arg store.CreateUserTxParams) (sqlcdb.User, error) {
			return sqlcdb.User{
				Username:  arg.Username,
				FullName:  arg.FullName,
				Email:     arg.Email,
				CreatedAt: time.Now(),
			}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"username":"alice","password":"` + testUserPassword + `","full_name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d (%s)", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["message"] != "check your email for verification instructions" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestCreateUserEmailExistsReturnsGenericAccepted(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		createUserTx: func(_ context.Context, arg store.CreateUserTxParams) (sqlcdb.User, error) {
			return sqlcdb.User{}, store.ErrEmailExists
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"username":"alice","password":"` + testUserPassword + `","full_name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202 for existing email, got %d (%s)", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["message"] != "check your email for verification instructions" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestCreateUserInternalTxErrorReturnsGenericFailure(t *testing.T) {
	t.Parallel()
	const internalErr = "river enqueue failed"
	fake := fakeStore{
		createUserTx: func(context.Context, store.CreateUserTxParams) (sqlcdb.User, error) {
			return sqlcdb.User{}, errors.New(internalErr)
		},
	}
	s := newTestServerWithStore(t, fake)
	var logBuf bytes.Buffer
	s.router.Logger = slog.New(slog.NewTextHandler(&logBuf, nil))

	body := `{"username":"alice","password":"` + testUserPassword + `","full_name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 for internal create-user transaction error, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := decodeErrorResponse(t, rec); got.Code != "internal_error" || got.Error != "internal server error" {
		t.Fatalf("unexpected error response: %+v", got)
	}
	logged := logBuf.String()
	if !strings.Contains(logged, "create user transaction") || !strings.Contains(logged, internalErr) {
		t.Fatalf("expected internal create-user error to be logged, got %q", logged)
	}
}

func TestCreateUserUsernameExistsReturnsGenericAccepted(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		createUserTx: func(context.Context, store.CreateUserTxParams) (sqlcdb.User, error) {
			return sqlcdb.User{}, store.ErrUsernameExists
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"username":"alice","password":"` + testUserPassword + `","full_name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202 for duplicate username, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateUserPasswordTooShort(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		createUserTx: func(context.Context, store.CreateUserTxParams) (sqlcdb.User, error) {
			t.Fatal("user must not be created for a short password")
			return sqlcdb.User{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)
	body := `{"username":"alice","password":"short-password","full_name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for short password, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateUserPasswordTooLong(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		createUserTx: func(context.Context, store.CreateUserTxParams) (sqlcdb.User, error) {
			t.Fatal("user must not be created for an over-long password")
			return sqlcdb.User{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	// 73 bytes exceeds bcrypt's 72-byte cap and must be rejected at validation
	// (400), not surfaced as a 500 from the hashing layer.
	longPassword := strings.Repeat("a", 73)
	body := `{"username":"alice","password":"` + longPassword + `","full_name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for over-long password, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestLoginUserOK(t *testing.T) {
	t.Parallel()
	hashed, err := password.Hash("secret123")
	if err != nil {
		t.Fatal(err)
	}
	var sessionCreated bool
	fake := fakeStore{
		getUser: func(_ context.Context, username string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: username, HashedPassword: hashed, Email: "alice@example.com", IsEmailVerified: true}, nil
		},
		createSession: func(_ context.Context, arg sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
			sessionCreated = true
			return sqlcdb.Session{ID: arg.ID, Username: arg.Username, RefreshToken: arg.RefreshToken, ExpiresAt: arg.ExpiresAt}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"username":"alice","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !sessionCreated {
		t.Fatal("login must create a session")
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["access_token"] == "" {
		t.Fatalf("expected access token in response: %+v", got)
	}
	if _, ok := got["refresh_token"]; ok {
		t.Fatal("login JSON exposed refresh token")
	}
	if _, ok := got["refresh_token_expires_at"]; ok {
		t.Fatal("login JSON exposed refresh expiry")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("want 1 cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != refreshCookieName || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe refresh cookie: %+v", cookie)
	}
	if cookie.Path != "/api/v1" {
		t.Fatalf("refresh cookie path = %q, want /api/v1", cookie.Path)
	}
	if !cookie.Secure {
		t.Fatalf("refresh cookie must be secure by default: %+v", cookie)
	}
	if cookie.Value == "" {
		t.Fatal("refresh cookie value must be set")
	}
}

func TestLogoutUserOK(t *testing.T) {
	t.Parallel()
	tokens := mustIssueTokenPair(t, "alice")
	var blocked uuid.UUID
	blockedCalled := false
	fake := fakeStore{
		blockSession: func(_ context.Context, id uuid.UUID) (sqlcdb.Session, error) {
			blockedCalled = true
			blocked = id
			return sqlcdb.Session{ID: id, IsBlocked: true}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/logout", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: tokens.refresh, Path: "/api/v1"})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !blockedCalled {
		t.Fatal("logout must block the session for a valid refresh cookie")
	}
	if blocked != tokens.refreshPayload.ID {
		t.Fatalf("blocked session ID = %s, want %s", blocked, tokens.refreshPayload.ID)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("want 1 cookie, got %d", len(cookies))
	}
	if got := cookies[0]; got.Name != refreshCookieName || got.Value != "" || got.MaxAge != -1 {
		t.Fatalf("logout must clear refresh cookie, got %+v", got)
	}
}

func TestLogoutUserWithoutCookie(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		blockSession: func(context.Context, uuid.UUID) (sqlcdb.Session, error) {
			t.Fatal("logout without a cookie must be idempotent")
			return sqlcdb.Session{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/logout", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d (%s)", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("want 1 cookie, got %d", len(cookies))
	}
	if got := cookies[0]; got.Name != refreshCookieName || got.Value != "" || got.MaxAge != -1 {
		t.Fatalf("logout must clear refresh cookie, got %+v", got)
	}
}

func TestLogoutUserClearsCookieWhenRevocationFails(t *testing.T) {
	t.Parallel()
	tokens := mustIssueTokenPair(t, "alice")
	fake := fakeStore{
		blockSession: func(context.Context, uuid.UUID) (sqlcdb.Session, error) {
			return sqlcdb.Session{}, errors.New("database unavailable")
		},
	}
	s := newTestServerWithStore(t, fake)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/logout", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: tokens.refresh, Path: "/api/v1"})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 when session revocation fails, got %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("revocation failure must still clear browser credential, got %+v", cookies)
	}
}

func mustIssueTokenPair(t *testing.T, username string) tokenPair {
	t.Helper()
	s := newTestServer(t)
	tokens, err := s.issueTokenPair(username, roleDepositor)
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

func TestLoginUserWrongPassword(t *testing.T) {
	t.Parallel()
	hashed, err := password.Hash("secret123")
	if err != nil {
		t.Fatal(err)
	}
	fake := fakeStore{
		getUser: func(_ context.Context, username string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: username, HashedPassword: hashed}, nil
		},
		createSession: func(context.Context, sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
			t.Fatal("session must not be created on a failed login")
			return sqlcdb.Session{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"username":"alice","password":"wrong-password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for wrong password, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := decodeErrorResponse(t, rec); got.Code != "invalid_credentials" {
		t.Fatalf("code = %q, want invalid_credentials", got.Code)
	}
}

func TestLoginUserUnknown(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{}, store.ErrRecordNotFound
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"username":"ghost","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	// Unknown user must be indistinguishable from wrong password: 401, not 404.
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for unknown user, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := decodeErrorResponse(t, rec); got.Code != "invalid_credentials" {
		t.Fatalf("code = %q, want invalid_credentials", got.Code)
	}
}

func TestLoginUserUnverified(t *testing.T) {
	t.Parallel()
	hashed, err := password.Hash("secret123")
	if err != nil {
		t.Fatal(err)
	}
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: "alice", HashedPassword: hashed, IsEmailVerified: false}, nil
		},
		createSession: func(context.Context, sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
			t.Fatal("unverified login must not create a session")
			return sqlcdb.Session{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"username":"alice","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := decodeErrorResponse(t, rec); got.Code != "email_verification_required" {
		t.Fatalf("code = %q, want email_verification_required", got.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("unverified login must not set cookies, got %d", len(rec.Result().Cookies()))
	}
}
