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
)

const testSecret = "01234567890123456789012345678901"

// fakeStore satisfies store.Store by embedding the interface (nil underlying).
// Only the methods a test actually exercises are overridden; calling any other
// method panics, which is the desired signal for an unexpected store access.
type fakeStore struct {
	store.Store
	createUserTx func(context.Context, store.CreateUserTxParams) (sqlcdb.User, error)
	getAccount   func(context.Context, uuid.UUID) (sqlcdb.Account, error)
	transferTx   func(context.Context, store.TransferTxParams) (store.TransferTxResult, error)
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

func newTestServer(t *testing.T) *Server {
	return newTestServerWithStore(t, nil)
}

func newTestServerWithStore(t *testing.T, st store.Store) *Server {
	t.Helper()
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(config.Config{JWTSecret: testSecret}, st, maker, nil)
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
