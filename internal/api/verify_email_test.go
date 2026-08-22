package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"uuid"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/secret"
)

func getVerifyEmail(t *testing.T, s *Server, id, code string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/verify_email?id="+id+"&code="+code, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func postResendVerifyEmail(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/verify_email/resend", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestVerifyEmailOK(t *testing.T) {
	t.Parallel()
	var called bool
	var gotArg store.VerifyEmailTxParams
	fake := fakeStore{
		verifyEmailTx: func(_ context.Context, arg store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error) {
			called = true
			gotArg = arg
			return store.VerifyEmailTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	id := uuid.New()
	const code = "a-secret-code"
	rec := getVerifyEmail(t, s, id.String(), code)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("VerifyEmailTx should run")
	}
	if gotArg.ID != id || gotArg.SecretCode != secret.Digest(code) {
		t.Fatalf("VerifyEmailTx params = %+v, want ID %s and hashed code", gotArg, id)
	}
}

func TestVerifyEmailBadID(t *testing.T) {
	t.Parallel()
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

func TestVerifyEmailEmptyCode(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		verifyEmailTx: func(context.Context, store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error) {
			t.Fatal("store must not be reached for an empty code")
			return store.VerifyEmailTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	if rec := getVerifyEmail(t, s, uuid.New().String(), ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for empty code, got %d", rec.Code)
	}
}

// TestVerifyEmailCodeNotLogged proves the request logger records the path only
// and never the query string, so the secret verification `code` cannot leak
// into logs.
func TestVerifyEmailCodeNotLogged(t *testing.T) {
	t.Parallel()
	const secret = "super-secret-verification-code"
	fake := fakeStore{
		verifyEmailTx: func(context.Context, store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error) {
			return store.VerifyEmailTxResult{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)

	var buf bytes.Buffer
	s.router.Logger = slog.New(slog.NewTextHandler(&buf, nil))

	rec := getVerifyEmail(t, s, uuid.New().String(), secret)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	logged := buf.String()
	if strings.Contains(logged, secret) {
		t.Fatalf("secret code leaked into request log: %q", logged)
	}
	if !strings.Contains(logged, "/api/v1/users/verify_email") {
		t.Fatalf("request path missing from log: %q", logged)
	}
}

func TestVerifyEmailInvalidCode(t *testing.T) {
	t.Parallel()
	fake := fakeStore{
		verifyEmailTx: func(context.Context, store.VerifyEmailTxParams) (store.VerifyEmailTxResult, error) {
			return store.VerifyEmailTxResult{}, store.ErrRecordNotFound
		},
	}
	s := newTestServerWithStore(t, fake)

	if rec := getVerifyEmail(t, s, uuid.New().String(), "wrong-code"); rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid/expired code, got %d", rec.Code)
	}
}

func TestResendVerifyEmailPrivacy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		user sqlcdb.User
		err  error
	}{
		{name: "unknown", err: store.ErrRecordNotFound},
		{name: "verified", user: sqlcdb.User{Username: "alice", Email: "alice@example.com", IsEmailVerified: true}},
		{name: "unverified", user: sqlcdb.User{Username: "alice", Email: "alice@example.com", IsEmailVerified: false}},
	}

	const body = `{"email":"alice@example.com"}`

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := fakeStore{
				getUserByEmail: func(context.Context, string) (sqlcdb.User, error) {
					return tc.user, tc.err
				},
			}
			s := newTestServerWithStore(t, fake)

			rec := postResendVerifyEmail(t, s, body)
			if rec.Code != http.StatusAccepted {
				t.Fatalf("want 202, got %d (%s)", rec.Code, rec.Body.String())
			}
			var got map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got["message"] != verificationAccepted["message"] {
				t.Fatalf("want generic body %q, got %q", verificationAccepted["message"], got["message"])
			}
		})
	}
}
