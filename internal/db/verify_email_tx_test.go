//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/util"
)

func TestVerifyEmailTx(t *testing.T) {
	u := createTestUser(t)
	code := util.RandomString(32)

	ve, err := testStore.CreateVerifyEmail(context.Background(), sqlcdb.CreateVerifyEmailParams{
		Username:   u.Username,
		Email:      u.Email,
		SecretCode: code,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wrong code must not verify anyone.
	if _, err := testStore.VerifyEmailTx(context.Background(), VerifyEmailTxParams{
		ID:         ve.ID,
		SecretCode: "wrong-code",
	}); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("want ErrRecordNotFound for wrong code, got %v", err)
	}

	// Correct code flips the user's verified flag.
	res, err := testStore.VerifyEmailTx(context.Background(), VerifyEmailTxParams{
		ID:         ve.ID,
		SecretCode: code,
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
	got, err := testStore.GetUser(context.Background(), u.Username)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsEmailVerified {
		t.Error("persisted user should be email-verified")
	}
}
