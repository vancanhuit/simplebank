package store

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrRecordNotFound      = errors.New("record not found")
	ErrUniqueViolation     = errors.New("unique constraint violation")
	ErrForeignKeyViolation = errors.New("foreign key violation")
	ErrInsufficientBalance = errors.New("insufficient balance")
)

func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRecordNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrUniqueViolation
		case "23503":
			return ErrForeignKeyViolation
		}
	}
	return err
}
