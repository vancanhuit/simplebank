//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/util"
)

func TestCreateUserTxRollbackOnAfterCreateError(t *testing.T) {
	hashed, err := util.HashPassword(util.RandomString(8))
	if err != nil {
		t.Fatal(err)
	}
	username := util.RandomOwner()
	boom := errors.New("enqueue failed")

	_, err = testStore.CreateUserTx(context.Background(), CreateUserTxParams{
		CreateUserParams: sqlcdb.CreateUserParams{
			Username:       username,
			HashedPassword: hashed,
			FullName:       util.RandomOwner(),
			Email:          util.RandomString(6) + "@example.com",
		},
		AfterCreate: func(tx pgx.Tx, user sqlcdb.User) error { return boom },
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want boom error, got %v", err)
	}

	// The user must NOT exist because the tx rolled back.
	if _, err := testStore.GetUser(context.Background(), username); !errors.Is(ClassifyError(err), ErrRecordNotFound) {
		t.Fatalf("expected user to be absent after rollback, got err=%v", err)
	}
}

func TestCreateUserTxCommit(t *testing.T) {
	hashed, err := util.HashPassword(util.RandomString(8))
	if err != nil {
		t.Fatal(err)
	}
	username := util.RandomOwner()

	var afterCalled bool
	user, err := testStore.CreateUserTx(context.Background(), CreateUserTxParams{
		CreateUserParams: sqlcdb.CreateUserParams{
			Username:       username,
			HashedPassword: hashed,
			FullName:       util.RandomOwner(),
			Email:          util.RandomString(6) + "@example.com",
		},
		AfterCreate: func(tx pgx.Tx, u sqlcdb.User) error {
			afterCalled = true
			if u.Username != username {
				t.Errorf("AfterCreate user = %q, want %q", u.Username, username)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("CreateUserTx: %v", err)
	}
	if !afterCalled {
		t.Fatal("AfterCreate was not called")
	}
	if user.Username != username {
		t.Fatalf("returned user = %q, want %q", user.Username, username)
	}

	// The user must exist because the tx committed.
	got, err := testStore.GetUser(context.Background(), username)
	if err != nil {
		t.Fatalf("user should exist after commit: %v", err)
	}
	if got.Username != username {
		t.Fatalf("persisted user = %q, want %q", got.Username, username)
	}
}
