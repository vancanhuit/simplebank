package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/token"
	"github.com/vancanhuit/simplebank/internal/util"
)

const testSecret = "01234567890123456789012345678901"

// fakeStore satisfies store.Store by embedding the interface (nil underlying).
// Only the methods a test actually exercises are overridden; calling any other
// method panics, which is the desired signal for an unexpected store access.
type fakeStore struct {
	store.Store
	createUserTx  func(context.Context, store.CreateUserTxParams) (sqlcdb.User, error)
	getAccount    func(context.Context, uuid.UUID) (sqlcdb.Account, error)
	transferTx    func(context.Context, store.TransferTxParams) (store.TransferTxResult, error)
	getUser       func(context.Context, string) (sqlcdb.User, error)
	createSession func(context.Context, sqlcdb.CreateSessionParams) (sqlcdb.Session, error)
	getSession    func(context.Context, uuid.UUID) (sqlcdb.Session, error)
	verifyEmailTx func(context.Context, store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error)
	listAccounts  func(context.Context, sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error)
	createAccount func(context.Context, sqlcdb.CreateAccountParams) (sqlcdb.Account, error)
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

func (f fakeStore) CreateSession(ctx context.Context, arg sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
	return f.createSession(ctx, arg)
}

func (f fakeStore) GetSession(ctx context.Context, id uuid.UUID) (sqlcdb.Session, error) {
	return f.getSession(ctx, id)
}

func (f fakeStore) VerifyEmailTx(ctx context.Context, arg store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error) {
	return f.verifyEmailTx(ctx, arg)
}

func (f fakeStore) ListAccounts(ctx context.Context, arg sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error) {
	return f.listAccounts(ctx, arg)
}

func (f fakeStore) CreateAccount(ctx context.Context, arg sqlcdb.CreateAccountParams) (sqlcdb.Account, error) {
	return f.createAccount(ctx, arg)
}

func newTestServer(t *testing.T) *Server {
	return newTestServerWithStore(t, nil)
}

func newTestServerWithStore(t *testing.T, st store.Store) *Server {
	t.Helper()
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		JWTSecret:  testSecret,
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	}
	s, err := NewServer(cfg, st, maker, nil)
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

func TestCreateUserOK(t *testing.T) {
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

	body := `{"username":"alice","password":"secret123","full_name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var got userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Username != "alice" || got.Email != "alice@example.com" {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	fake := fakeStore{
		createUserTx: func(context.Context, store.CreateUserTxParams) (sqlcdb.User, error) {
			return sqlcdb.User{}, store.ErrUniqueViolation
		},
	}
	s := newTestServerWithStore(t, fake)

	body := `{"username":"alice","password":"secret123","full_name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409 for duplicate username, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateUserPasswordTooLong(t *testing.T) {
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
	hashed, err := util.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	var sessionCreated bool
	fake := fakeStore{
		getUser: func(_ context.Context, username string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: username, HashedPassword: hashed, Email: "alice@example.com"}, nil
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
	var got loginUserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AccessToken == "" || got.RefreshToken == "" {
		t.Fatalf("expected tokens in response: %+v", got)
	}
}

func TestLoginUserWrongPassword(t *testing.T) {
	hashed, err := util.HashPassword("secret123")
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

func TestLoginUserUnknown(t *testing.T) {
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
}
