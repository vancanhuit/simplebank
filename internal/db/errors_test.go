package store

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestClassifyError(t *testing.T) {
	if !errors.Is(ClassifyError(pgx.ErrNoRows), ErrRecordNotFound) {
		t.Error("ErrNoRows should map to ErrRecordNotFound")
	}
	uniq := &pgconn.PgError{Code: "23505"}
	if !errors.Is(ClassifyError(uniq), ErrUniqueViolation) {
		t.Error("23505 should map to ErrUniqueViolation")
	}
	username := &pgconn.PgError{Code: "23505", ConstraintName: "users_username_key"}
	if !errors.Is(ClassifyError(username), ErrUsernameExists) {
		t.Error("users_username_key should map to ErrUsernameExists")
	}
	email := &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"}
	if !errors.Is(ClassifyError(email), ErrEmailExists) {
		t.Error("users_email_key should map to ErrEmailExists")
	}
	fk := &pgconn.PgError{Code: "23503"}
	if !errors.Is(ClassifyError(fk), ErrForeignKeyViolation) {
		t.Error("23503 should map to ErrForeignKeyViolation")
	}
	balanceCheck := &pgconn.PgError{Code: "23514", ConstraintName: "accounts_balance_javascript_safe"}
	if !errors.Is(ClassifyError(balanceCheck), ErrBalanceLimitExceeded) {
		t.Error("23514 accounts_balance_javascript_safe should map to ErrBalanceLimitExceeded")
	}
	other := errors.New("boom")
	if ClassifyError(other) != other {
		t.Error("unknown error should pass through unchanged")
	}
	if ClassifyError(nil) != nil {
		t.Error("nil should classify as nil")
	}
}

func TestErrInvalidSession(t *testing.T) {
	// ErrInvalidSession is a sentinel error returned directly, not classified
	if !errors.Is(ErrInvalidSession, ErrInvalidSession) {
		t.Error("ErrInvalidSession should match itself")
	}
	// Ensure it doesn't expose hash values
	msg := ErrInvalidSession.Error()
	if msg != "invalid session" {
		t.Errorf("ErrInvalidSession message = %q, want %q", msg, "invalid session")
	}
}
