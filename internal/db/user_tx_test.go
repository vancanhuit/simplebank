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
	hashed, _ := util.HashPassword(util.RandomString(8))
	username := util.RandomOwner()
	boom := errors.New("enqueue failed")

	_, err := testStore.CreateUserTx(context.Background(), CreateUserTxParams{
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
