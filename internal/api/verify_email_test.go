package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	store "github.com/vancanhuit/simplebank/internal/db"
)

func getVerifyEmail(t *testing.T, s *Server, id, code string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/verify_email?id="+id+"&code="+code, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestVerifyEmailOK(t *testing.T) {
	var called bool
	fake := fakeStore{
		verifyEmailTx: func(context.Context, store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error) {
			called = true
			return store.VerifyEmailTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	rec := getVerifyEmail(t, s, uuid.NewString(), "a-secret-code")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("VerifyEmailTx should run")
	}
}

func TestVerifyEmailBadID(t *testing.T) {
	fake := fakeStore{
		verifyEmailTx: func(context.Context, store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error) {
			t.Fatal("store must not be reached for a malformed id")
			return store.VerifyEmailTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	if rec := getVerifyEmail(t, s, "not-a-uuid", "code"); rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for malformed id, got %d", rec.Code)
	}
}

func TestVerifyEmailInvalidCode(t *testing.T) {
	fake := fakeStore{
		verifyEmailTx: func(context.Context, store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error) {
			return store.VerifyEmailTxResult{}, store.ErrRecordNotFound
		},
	}
	s := newTestServerWithStore(t, fake)

	if rec := getVerifyEmail(t, s, uuid.NewString(), "wrong-code"); rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid/expired code, got %d", rec.Code)
	}
}
