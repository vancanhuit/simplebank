package store

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"
	"uuid"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

// SessionReplacement contains the new refresh token parameters for rotation.
// RotateSessionTx preserves the existing session row ID; ID is the stable
// signed refresh JWT/session ID the caller expects for the rotated token.
type SessionReplacement struct {
	ID               uuid.UUID
	RefreshTokenHash string
	ExpiresAt        time.Time
}

// RotateSessionTxParams holds the parameters for atomic session rotation.
type RotateSessionTxParams struct {
	ID               uuid.UUID
	Username         string
	RefreshTokenHash string
	Now              time.Time
	NewSession       func() (SessionReplacement, error)
}

// RotateSessionTx atomically validates and rotates a refresh session in place.
// It locks the session row, validates all constraints (username, token hash,
// not blocked, not expired), and only then calls NewSession to generate
// replacement parameters. This ensures exactly-once rotation under concurrency.
func (s *SQLStore) RotateSessionTx(ctx context.Context, arg RotateSessionTxParams) (sqlcdb.Session, error) {
	var rotated sqlcdb.Session
	err := s.execTx(ctx, func(q *sqlcdb.Queries) error {
		// Lock the session row for update
		session, err := q.GetSessionForUpdate(ctx, arg.ID)
		if err != nil {
			if errors.Is(ClassifyError(err), ErrRecordNotFound) {
				return ErrInvalidSession
			}
			return ClassifyError(err)
		}

		// Validate session constraints using constant-time comparison for token hash
		tokenMatches := subtle.ConstantTimeCompare(
			[]byte(session.RefreshToken), []byte(arg.RefreshTokenHash),
		) == 1

		if session.IsBlocked || session.Username != arg.Username ||
			!tokenMatches || !session.ExpiresAt.After(arg.Now) {
			return ErrInvalidSession
		}

		// Generate replacement session parameters only after validation
		replacement, err := arg.NewSession()
		if err != nil {
			return err
		}
		if replacement.ID != session.ID {
			return ErrSessionIDMismatch
		}

		// Rotate the session
		rotated, err = q.RotateSession(ctx, sqlcdb.RotateSessionParams{
			OldID:           session.ID,
			NewRefreshToken: replacement.RefreshTokenHash,
			NewExpiresAt:    replacement.ExpiresAt,
		})
		return ClassifyError(err)
	})
	return rotated, err
}
