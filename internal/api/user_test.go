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

	"github.com/google/uuid"

	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/password"
	"github.com/vancanhuit/simplebank/internal/token"
)

const testSecret = "01234567890123456789012345678901"
const testPassword = "correct horse battery staple"

func createUserBody(username, password, fullName, email string) string {
	return `{"username":"` + username + `","password":"` + password + `","full_name":"` + fullName + `","email":"` + email + `"}`
}

func loginBody(username, password string) string {
	return `{"username":"` + username + `","password":"` + password + `"}`
}

// fakeStore satisfies store.Store by embedding the interface (nil underlying).
// Only the methods a test actually exercises are overridden; calling any other
// method panics, which is the desired signal for an unexpected store access.
type fakeStore struct {
	store.Store
	createUserTx              func(context.Context, store.CreateUserTxParams) (sqlcdb.User, error)
	getAccount                func(context.Context, uuid.UUID) (sqlcdb.Account, error)
	transferTx                func(context.Context, store.TransferTxParams) (store.TransferTxResult, error)
	getUser                   func(context.Context, string) (sqlcdb.User, error)
	getUserByEmail            func(context.Context, string) (sqlcdb.User, error)
	createSession             func(context.Context, sqlcdb.CreateSessionParams) (sqlcdb.Session, error)
	getSession                func(context.Context, uuid.UUID) (sqlcdb.Session, error)
	rotateSessionTx           func(context.Context, store.RotateSessionTxParams) (sqlcdb.Session, error)
	blockSession              func(context.Context, uuid.UUID) (sqlcdb.Session, error)
	verifyEmailTx             func(context.Context, store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error)
	listAccounts              func(context.Context, sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error)
	createAccountTx           func(context.Context, sqlcdb.CreateAccountParams) (sqlcdb.Account, error)
	listTransfers             func(context.Context, sqlcdb.ListTransfersByAccountParams) ([]sqlcdb.Transfer, error)
	checkLoginThrottle        func(context.Context, string, string, time.Time) (store.LoginThrottleDecision, error)
	recordLoginFailure        func(context.Context, string, string, time.Time) (store.LoginThrottleDecision, error)
	validateAccessSession     func(context.Context, uuid.UUID, string, time.Time) error
	clearLoginAccountThrottle func(context.Context, string) error
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

func (f fakeStore) CheckLoginThrottle(
	ctx context.Context,
	username string,
	clientIP string,
	now time.Time,
) (store.LoginThrottleDecision, error) {
	if f.checkLoginThrottle == nil {
		return store.LoginThrottleDecision{}, nil
	}
	return f.checkLoginThrottle(ctx, username, clientIP, now)
}

func (f fakeStore) RecordLoginFailure(
	ctx context.Context,
	username string,
	clientIP string,
	now time.Time,
) (store.LoginThrottleDecision, error) {
	if f.recordLoginFailure == nil {
		return store.LoginThrottleDecision{}, nil
	}
	return f.recordLoginFailure(ctx, username, clientIP, now)
}

func (f fakeStore) ClearLoginAccountThrottle(ctx context.Context, username string) error {
	if f.clearLoginAccountThrottle == nil {
		return nil
	}
	return f.clearLoginAccountThrottle(ctx, username)
}

func (f fakeStore) ValidateAccessSession(
	ctx context.Context,
	id uuid.UUID,
	username string,
	now time.Time,
) error {
	if f.validateAccessSession == nil {
		return nil
	}
	return f.validateAccessSession(ctx, id, username, now)
}

func newTestServer(t *testing.T) *Server {
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
	s, err := NewServer(cfg, st, maker, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newLoginRequest(body string, clientIP string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = clientIP + ":1234"
	return req
}

func assertLoginThrottleResponse(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q, want 30", got)
	}
	if !strings.Contains(rec.Body.String(), "too many login attempts") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
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

	body := createUserBody("alice", testPassword, "Alice", "alice@example.com")
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

	body := createUserBody("alice", testPassword, "Alice", "alice@example.com")
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

func TestCreateUserInternalTxErrorReturnsGenericAccepted(t *testing.T) {
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

	body := createUserBody("alice", testPassword, "Alice", "alice@example.com")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202 for internal create-user transaction error, got %d (%s)", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["message"] != "check your email for verification instructions" {
		t.Fatalf("unexpected response: %+v", got)
	}
	logged := logBuf.String()
	if !strings.Contains(logged, "create user transaction") || !strings.Contains(logged, internalErr) {
		t.Fatalf("expected internal create-user error to be logged, got %q", logged)
	}
}

func TestCreateUserPasswordFourteenBytesRejected(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		createUserTx: func(context.Context, store.CreateUserTxParams) (sqlcdb.User, error) {
			t.Fatal("user must not be created for a 14-byte password")
			return sqlcdb.User{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	body := createUserBody("alice", strings.Repeat("a", 14), "Alice", "alice@example.com")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for 14-byte password, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateUserPasswordFifteenBytesAccepted(t *testing.T) {
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

	body := createUserBody("alice", strings.Repeat("a", 15), "Alice", "alice@example.com")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202 for 15-byte password, got %d (%s)", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["message"] != "check your email for verification instructions" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestCreateUserMultibytePasswordAccepted(t *testing.T) {
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

	body := createUserBody("alice", strings.Repeat("é", 8), "Alice", "alice@example.com")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202 for multibyte password, got %d (%s)", rec.Code, rec.Body.String())
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

	body := createUserBody("alice", testPassword, "Alice", "alice@example.com")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202 for duplicate username, got %d (%s)", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["message"] != "check your email for verification instructions" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestCreateUserUsernameAndEmailResponsesMatch(t *testing.T) {
	t.Parallel()
	call := func(err error) (int, string) {
		t.Helper()
		fake := fakeStore{
			createUserTx: func(context.Context, store.CreateUserTxParams) (sqlcdb.User, error) {
				return sqlcdb.User{}, err
			},
		}
		s := newTestServerWithStore(t, fake)
		body := createUserBody("alice", testPassword, "Alice", "alice@example.com")
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	usernameCode, usernameBody := call(store.ErrUsernameExists)
	emailCode, emailBody := call(store.ErrEmailExists)

	if usernameCode != http.StatusAccepted || emailCode != http.StatusAccepted {
		t.Fatalf("want 202/202, got %d/%d", usernameCode, emailCode)
	}
	if usernameBody != emailBody {
		t.Fatalf("responses differ:\nusername=%s\nemail=%s", usernameBody, emailBody)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(usernameBody), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["message"] != "check your email for verification instructions" {
		t.Fatalf("unexpected response body: %s", usernameBody)
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
	body := createUserBody("alice", longPassword, "Alice", "alice@example.com")
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
	hashed, err := password.Hash(testPassword)
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

	body := loginBody("alice", testPassword)
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

func TestLogoutImmediatelyRevokesAccessToken(t *testing.T) {
	t.Parallel()
	tokens := mustIssueTokenPair(t, "alice")
	protectedCalls := 0
	blocked := false
	fake := fakeStore{
		validateAccessSession: func(_ context.Context, id uuid.UUID, username string, now time.Time) error {
			if id != tokens.refreshPayload.ID {
				return store.ErrInvalidSession
			}
			if username != "alice" {
				t.Fatalf("username = %q, want alice", username)
			}
			if now.IsZero() {
				t.Fatal("ValidateAccessSession must receive current time")
			}
			if blocked {
				return store.ErrInvalidSession
			}
			return nil
		},
		listAccounts: func(context.Context, sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error) {
			protectedCalls++
			return []sqlcdb.Account{}, nil
		},
		blockSession: func(_ context.Context, id uuid.UUID) (sqlcdb.Session, error) {
			if id != tokens.refreshPayload.ID {
				t.Fatalf("blocked session ID = %s, want %s", id, tokens.refreshPayload.ID)
			}
			blocked = true
			return sqlcdb.Session{ID: id, Username: "alice", IsBlocked: true}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	protectedReq := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	protectedReq.Header.Set("Authorization", "Bearer "+tokens.access)
	protectedRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(protectedRec, protectedReq)
	if protectedRec.Code != http.StatusOK {
		t.Fatalf("want 200 before logout, got %d (%s)", protectedRec.Code, protectedRec.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/users/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: refreshCookieName, Value: tokens.refresh, Path: "/api/v1"})
	logoutRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("want 204 logout, got %d (%s)", logoutRec.Code, logoutRec.Body.String())
	}

	protectedRec = httptest.NewRecorder()
	s.Handler().ServeHTTP(protectedRec, protectedReq)
	if protectedRec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 after logout, got %d (%s)", protectedRec.Code, protectedRec.Body.String())
	}
	if protectedCalls != 1 {
		t.Fatalf("protected handler calls = %d, want 1 before revocation only", protectedCalls)
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

func mustIssueTokenPair(t *testing.T, username string) tokenPair {
	t.Helper()
	s := newTestServer(t)
	tokens, err := s.issueTokenPair(username, roleDepositor)
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

func TestIssueTokenPairUsesOneSessionID(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	tokens, err := s.issueTokenPair("alice", roleDepositor)
	if err != nil {
		t.Fatal(err)
	}
	if tokens.accessPayload.ID != tokens.refreshPayload.ID {
		t.Fatalf("payload IDs differ: access %s refresh %s", tokens.accessPayload.ID, tokens.refreshPayload.ID)
	}
	accessPayload, err := s.tokenMaker.VerifyToken(tokens.access, token.Access)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	refreshPayload, err := s.tokenMaker.VerifyToken(tokens.refresh, token.Refresh)
	if err != nil {
		t.Fatalf("verify refresh token: %v", err)
	}
	if accessPayload.ID != refreshPayload.ID {
		t.Fatalf("token IDs differ: access %s refresh %s", accessPayload.ID, refreshPayload.ID)
	}
}

func TestLoginUserWrongPassword(t *testing.T) {
	t.Parallel()
	hashed, err := password.Hash(testPassword)
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
}

func TestLoginUserRejectsActiveAccountThrottle(t *testing.T) {
	t.Parallel()
	const clientIP = "198.51.100.10"
	fake := fakeStore{
		checkLoginThrottle: func(_ context.Context, username string, gotClientIP string, now time.Time) (store.LoginThrottleDecision, error) {
			if username != "alice" {
				t.Fatalf("username = %q, want alice", username)
			}
			if gotClientIP != clientIP {
				t.Fatalf("client IP = %q, want %q", gotClientIP, clientIP)
			}
			if now.IsZero() {
				t.Fatal("throttle check must receive current time")
			}
			return store.LoginThrottleDecision{RetryAfter: 30 * time.Second}, nil
		},
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			t.Fatal("active account throttle must reject before user lookup")
			return sqlcdb.User{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, newLoginRequest(loginBody("alice", testPassword), clientIP))

	assertLoginThrottleResponse(t, rec)
}

func TestLoginUserRejectsActiveClientThrottle(t *testing.T) {
	t.Parallel()
	const clientIP = "198.51.100.11"
	fake := fakeStore{
		checkLoginThrottle: func(_ context.Context, username string, gotClientIP string, _ time.Time) (store.LoginThrottleDecision, error) {
			if username != "alice" {
				t.Fatalf("username = %q, want alice", username)
			}
			if gotClientIP != clientIP {
				t.Fatalf("client IP = %q, want %q", gotClientIP, clientIP)
			}
			return store.LoginThrottleDecision{RetryAfter: 30 * time.Second}, nil
		},
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			t.Fatal("active client throttle must reject before user lookup")
			return sqlcdb.User{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, newLoginRequest(loginBody("alice", testPassword), clientIP))

	assertLoginThrottleResponse(t, rec)
}

func TestLoginUserRecordsUnknownUserFailure(t *testing.T) {
	t.Parallel()
	const clientIP = "198.51.100.12"
	recorded := false
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{}, store.ErrRecordNotFound
		},
		recordLoginFailure: func(_ context.Context, username string, gotClientIP string, now time.Time) (store.LoginThrottleDecision, error) {
			recorded = true
			if username != "ghost" {
				t.Fatalf("username = %q, want ghost", username)
			}
			if gotClientIP != clientIP {
				t.Fatalf("client IP = %q, want %q", gotClientIP, clientIP)
			}
			if now.IsZero() {
				t.Fatal("record failure must receive current time")
			}
			return store.LoginThrottleDecision{}, nil
		},
		clearLoginAccountThrottle: func(context.Context, string) error {
			t.Fatal("unknown user must not clear account throttle")
			return nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, newLoginRequest(loginBody("ghost", testPassword), clientIP))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for unknown user, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !recorded {
		t.Fatal("unknown user must record a login failure")
	}
}

func TestLoginUserRecordsWrongPasswordFailure(t *testing.T) {
	t.Parallel()
	hashed, err := password.Hash(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	const clientIP = "198.51.100.13"
	recorded := false
	fake := fakeStore{
		getUser: func(_ context.Context, username string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: username, HashedPassword: hashed}, nil
		},
		recordLoginFailure: func(_ context.Context, username string, gotClientIP string, now time.Time) (store.LoginThrottleDecision, error) {
			recorded = true
			if username != "alice" {
				t.Fatalf("username = %q, want alice", username)
			}
			if gotClientIP != clientIP {
				t.Fatalf("client IP = %q, want %q", gotClientIP, clientIP)
			}
			if now.IsZero() {
				t.Fatal("record failure must receive current time")
			}
			return store.LoginThrottleDecision{}, nil
		},
		createSession: func(context.Context, sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
			t.Fatal("session must not be created on a failed login")
			return sqlcdb.Session{}, nil
		},
		clearLoginAccountThrottle: func(context.Context, string) error {
			t.Fatal("wrong password must not clear account throttle")
			return nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, newLoginRequest(`{"username":"alice","password":"wrong-password"}`, clientIP))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for wrong password, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !recorded {
		t.Fatal("wrong password must record a login failure")
	}
}

func TestLoginUserReturns429WhenFailureStartsCooldown(t *testing.T) {
	t.Parallel()
	hashed, err := password.Hash(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	const clientIP = "198.51.100.14"
	fake := fakeStore{
		getUser: func(_ context.Context, username string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: username, HashedPassword: hashed}, nil
		},
		recordLoginFailure: func(_ context.Context, username string, gotClientIP string, _ time.Time) (store.LoginThrottleDecision, error) {
			if username != "alice" {
				t.Fatalf("username = %q, want alice", username)
			}
			if gotClientIP != clientIP {
				t.Fatalf("client IP = %q, want %q", gotClientIP, clientIP)
			}
			return store.LoginThrottleDecision{RetryAfter: 30 * time.Second}, nil
		},
		createSession: func(context.Context, sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
			t.Fatal("session must not be created when cooldown starts")
			return sqlcdb.Session{}, nil
		},
		clearLoginAccountThrottle: func(context.Context, string) error {
			t.Fatal("failed login must not clear account throttle")
			return nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, newLoginRequest(`{"username":"alice","password":"wrong-password"}`, clientIP))

	assertLoginThrottleResponse(t, rec)
}

func TestLoginUserUnknown(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{}, store.ErrRecordNotFound
		},
	}
	s := newTestServerWithStore(t, fake)

	body := loginBody("ghost", testPassword)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	// Unknown user must be indistinguishable from wrong password: 401, not 404.
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for unknown user, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestLoginUserUnverified(t *testing.T) {
	t.Parallel()
	hashed, err := password.Hash(testPassword)
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

	body := loginBody("alice", testPassword)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("unverified login must not set cookies, got %d", len(rec.Result().Cookies()))
	}
}

func TestLoginUserClearsAccountThrottleAfterValidCredentials(t *testing.T) {
	t.Parallel()
	hashed, err := password.Hash(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		isVerified bool
		wantStatus int
	}{
		{name: "verified user", isVerified: true, wantStatus: http.StatusOK},
		{name: "unverified user", isVerified: false, wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cleared := false
			fake := fakeStore{
				getUser: func(context.Context, string) (sqlcdb.User, error) {
					return sqlcdb.User{
						Username:        "alice",
						HashedPassword:  hashed,
						IsEmailVerified: tt.isVerified,
					}, nil
				},
				clearLoginAccountThrottle: func(_ context.Context, username string) error {
					cleared = true
					if username != "alice" {
						t.Fatalf("username = %q, want alice", username)
					}
					return nil
				},
				createSession: func(_ context.Context, arg sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
					if !tt.isVerified {
						t.Fatal("unverified login must not create a session")
					}
					return sqlcdb.Session{ID: arg.ID, Username: arg.Username, RefreshToken: arg.RefreshToken, ExpiresAt: arg.ExpiresAt}, nil
				},
			}
			s := newTestServerWithStore(t, fake)

			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, newLoginRequest(loginBody("alice", testPassword), "198.51.100.15"))

			if rec.Code != tt.wantStatus {
				t.Fatalf("want %d, got %d (%s)", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if !cleared {
				t.Fatal("valid credentials must clear account throttle")
			}
		})
	}
}

func TestLoginUserDoesNotClearClientThrottle(t *testing.T) {
	t.Parallel()
	hashed, err := password.Hash(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	const clientIP = "198.51.100.16"
	var clearedUsername string
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{
				Username:        "alice",
				HashedPassword:  hashed,
				IsEmailVerified: true,
			}, nil
		},
		clearLoginAccountThrottle: func(_ context.Context, username string) error {
			clearedUsername = username
			return nil
		},
		createSession: func(_ context.Context, arg sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
			return sqlcdb.Session{ID: arg.ID, Username: arg.Username, RefreshToken: arg.RefreshToken, ExpiresAt: arg.ExpiresAt}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, newLoginRequest(loginBody("alice", testPassword), clientIP))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if clearedUsername != "alice" {
		t.Fatalf("cleared username = %q, want alice", clearedUsername)
	}
	if clearedUsername == clientIP {
		t.Fatalf("login must clear account throttle, not client throttle %q", clientIP)
	}
}

func TestLoginUserThrottleStoreErrorReturns500(t *testing.T) {
	t.Parallel()
	hashed, err := password.Hash(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		fake fakeStore
		body string
	}{
		{
			name: "check login throttle",
			fake: fakeStore{
				checkLoginThrottle: func(context.Context, string, string, time.Time) (store.LoginThrottleDecision, error) {
					return store.LoginThrottleDecision{}, errors.New("check throttle failed")
				},
				getUser: func(context.Context, string) (sqlcdb.User, error) {
					t.Fatal("user lookup must not run when throttle check fails")
					return sqlcdb.User{}, nil
				},
			},
			body: loginBody("alice", testPassword),
		},
		{
			name: "record login failure",
			fake: fakeStore{
				getUser: func(_ context.Context, username string) (sqlcdb.User, error) {
					return sqlcdb.User{Username: username, HashedPassword: hashed}, nil
				},
				recordLoginFailure: func(context.Context, string, string, time.Time) (store.LoginThrottleDecision, error) {
					return store.LoginThrottleDecision{}, errors.New("record failure failed")
				},
				createSession: func(context.Context, sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
					t.Fatal("session must not be created when failure recording fails")
					return sqlcdb.Session{}, nil
				},
			},
			body: `{"username":"alice","password":"wrong-password"}`,
		},
		{
			name: "clear account throttle",
			fake: fakeStore{
				getUser: func(context.Context, string) (sqlcdb.User, error) {
					return sqlcdb.User{
						Username:        "alice",
						HashedPassword:  hashed,
						IsEmailVerified: true,
					}, nil
				},
				clearLoginAccountThrottle: func(context.Context, string) error {
					return errors.New("clear account throttle failed")
				},
				createSession: func(context.Context, sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
					t.Fatal("session must not be created when clearing throttle fails")
					return sqlcdb.Session{}, nil
				},
			},
			body: loginBody("alice", testPassword),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newTestServerWithStore(t, tt.fake)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, newLoginRequest(tt.body, "198.51.100.17"))

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("want 500, got %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "internal server error") {
				t.Fatalf("unexpected body: %s", rec.Body.String())
			}
		})
	}
}
