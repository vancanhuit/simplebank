package store

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrRecordNotFound      = errors.New("record not found")
	ErrUsernameExists      = errors.New("username already exists")
	ErrEmailExists         = errors.New("email already exists")
	ErrUniqueViolation     = errors.New("unique constraint violation")
	ErrForeignKeyViolation = errors.New("foreign key violation")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrCurrencyMismatch    = errors.New("currency mismatch")
	ErrNumericOutOfRange   = errors.New("numeric value out of range")
	ErrDailyLimitExceeded  = errors.New("daily transfer limit exceeded")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different transfer parameters")
	ErrInvalidSession      = errors.New("invalid session")
	ErrSessionIDMismatch   = errors.New("session replacement id mismatch")
)

func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRecordNotFound
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "23505":
			switch pgErr.ConstraintName {
			case "users_username_key":
				return ErrUsernameExists
			case "users_email_key":
				return ErrEmailExists
			default:
				return ErrUniqueViolation
			}
		case "23503":
			return ErrForeignKeyViolation
		case "22003":
			return ErrNumericOutOfRange
		}
	}
	return err
}
