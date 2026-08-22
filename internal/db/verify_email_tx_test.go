//go:build integration

package store

import (
	"errors"
	"testing"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/random"
	"github.com/vancanhuit/simplebank/internal/secret"
)

func TestVerifyEmailTx(t *testing.T) {
	u := createTestUser(t)
	code := random.String(32)

	ve, err := testStore.CreateVerifyEmail(t.Context(), sqlcdb.CreateVerifyEmailParams{
		Username:   u.Username,
		Email:      u.Email,
		SecretCode: secret.Digest(code),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wrong code must not verify anyone.
	if _, err := testStore.VerifyEmailTx(t.Context(), VerifyEmailTxParams{
		ID:         ve.ID,
		SecretCode: "wrong-code",
	}); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("want ErrRecordNotFound for wrong code, got %v", err)
	}

	// Correct code flips the user's verified flag.
	res, err := testStore.VerifyEmailTx(t.Context(), VerifyEmailTxParams{
		ID:         ve.ID,
		SecretCode: secret.Digest(code),
	})
	if err != nil {
		t.Fatalf("VerifyEmailTx: %v", err)
	}
	if res.User.Username != u.Username {
		t.Errorf("verified user = %q, want %q", res.User.Username, u.Username)
	}
	if !res.User.IsEmailVerified {
		t.Error("user should be marked email-verified")
	}

	// Confirmed persisted.
	got, err := testStore.GetUser(t.Context(), u.Username)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsEmailVerified {
		t.Error("persisted user should be email-verified")
	}
}
