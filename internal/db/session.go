package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ValidateAccessSession checks whether a session-backed access token still maps
// to a live, unblocked session row for the same user.
func (s *SQLStore) ValidateAccessSession(
	ctx context.Context,
	id uuid.UUID,
	username string,
	now time.Time,
) error {
	session, err := s.GetSession(ctx, id)
	if err != nil {
		classified := ClassifyError(err)
		if errors.Is(classified, ErrRecordNotFound) {
			return ErrInvalidSession
		}
		return classified
	}
	if session.IsBlocked || session.Username != username || !session.ExpiresAt.After(now) {
		return ErrInvalidSession
	}
	return nil
}
