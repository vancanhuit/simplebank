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
	fk := &pgconn.PgError{Code: "23503"}
	if !errors.Is(ClassifyError(fk), ErrForeignKeyViolation) {
		t.Error("23503 should map to ErrForeignKeyViolation")
	}
	other := errors.New("boom")
	if ClassifyError(other) != other {
		t.Error("unknown error should pass through unchanged")
	}
	if ClassifyError(nil) != nil {
		t.Error("nil should classify as nil")
	}
}
