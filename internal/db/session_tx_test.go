//go:build integration

package store

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

func TestRotateSessionTx_Valid(t *testing.T) {
	user := createTestUser(t)
	oldID := uuid.New()
	const oldHash = "old-refresh-hash"

	_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
		ID:           oldID,
		Username:     user.Username,
		RefreshToken: oldHash,
		UserAgent:    "test-agent",
		ClientIp:     "127.0.0.1",
		IsBlocked:    false,
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	newHash := uuid.NewString()
	newExpiry := time.Now().Add(2 * time.Hour)

	rotated, err := testStore.RotateSessionTx(t.Context(), RotateSessionTxParams{
		ID:               oldID,
		Username:         user.Username,
		RefreshTokenHash: oldHash,
		Now:              time.Now(),
		NewSession: func() (SessionReplacement, error) {
			return SessionReplacement{
				ID:               oldID,
				RefreshTokenHash: newHash,
				ExpiresAt:        newExpiry,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("RotateSessionTx failed: %v", err)
	}

	if rotated.ID != oldID {
		t.Errorf("rotated.ID = %v, want stable %v", rotated.ID, oldID)
	}
	if rotated.RefreshToken != newHash {
		t.Errorf("rotated.RefreshToken = %v, want %v", rotated.RefreshToken, newHash)
	}
	if rotated.Username != user.Username {
		t.Errorf("rotated.Username = %v, want %v", rotated.Username, user.Username)
	}

	got, err := testStore.GetSession(t.Context(), oldID)
	if err != nil {
		t.Fatalf("stable session should exist: %v", err)
	}
	if got.ID != oldID {
		t.Errorf("persisted session ID = %v, want stable %v", got.ID, oldID)
	}
	if got.RefreshToken != newHash {
		t.Errorf("persisted refresh hash = %v, want %v", got.RefreshToken, newHash)
	}
}

func TestRotateSessionTx_ReplacementIDMismatch(t *testing.T) {
	user := createTestUser(t)
	oldID := uuid.New()
	const oldHash = "old-refresh-hash"

	_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
		ID:           oldID,
		Username:     user.Username,
		RefreshToken: oldHash,
		UserAgent:    "test-agent",
		ClientIp:     "127.0.0.1",
		IsBlocked:    false,
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = testStore.RotateSessionTx(t.Context(), RotateSessionTxParams{
		ID:               oldID,
		Username:         user.Username,
		RefreshTokenHash: oldHash,
		Now:              time.Now(),
		NewSession: func() (SessionReplacement, error) {
			return SessionReplacement{
				ID:               uuid.New(),
				RefreshTokenHash: uuid.NewString(),
				ExpiresAt:        time.Now().Add(2 * time.Hour),
			}, nil
		},
	})
	if !errors.Is(err, ErrSessionIDMismatch) {
		t.Fatalf("want ErrSessionIDMismatch, got %v", err)
	}

	got, err := testStore.GetSession(t.Context(), oldID)
	if err != nil {
		t.Fatalf("get unchanged session: %v", err)
	}
	if got.RefreshToken != oldHash {
		t.Fatalf("refresh hash changed on mismatch: got %q, want %q", got.RefreshToken, oldHash)
	}
	if got.IsBlocked {
		t.Fatal("session should not be blocked by replacement ID mismatch")
	}
}

func TestRotateSessionTx_BlockedSession(t *testing.T) {
	user := createTestUser(t)
	oldID := uuid.New()
	const oldHash = "old-refresh-hash"

	_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
		ID:           oldID,
		Username:     user.Username,
		RefreshToken: oldHash,
		UserAgent:    "test-agent",
		ClientIp:     "127.0.0.1",
		IsBlocked:    true, // Session is blocked
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = testStore.RotateSessionTx(t.Context(), RotateSessionTxParams{
		ID:               oldID,
		Username:         user.Username,
		RefreshTokenHash: oldHash,
		Now:              time.Now(),
		NewSession: func() (SessionReplacement, error) {
			return SessionReplacement{
				ID:               uuid.New(),
				RefreshTokenHash: uuid.NewString(),
				ExpiresAt:        time.Now().Add(time.Hour),
			}, nil
		},
	})
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession, got %v", err)
	}
}

func TestRotateSessionTx_ExpiredSession(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	testCases := []struct {
		name      string
		expiresAt time.Time
	}{
		{
			name:      "already expired",
			expiresAt: now.Add(-time.Hour),
		},
		{
			name:      "expires exactly now (boundary)",
			expiresAt: now,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			user := createTestUser(t)
			oldID := uuid.New()
			const oldHash = "old-refresh-hash"

			_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
				ID:           oldID,
				Username:     user.Username,
				RefreshToken: oldHash,
				UserAgent:    "test-agent",
				ClientIp:     "127.0.0.1",
				IsBlocked:    false,
				ExpiresAt:    tc.expiresAt,
			})
			if err != nil {
				t.Fatal(err)
			}

			_, err = testStore.RotateSessionTx(t.Context(), RotateSessionTxParams{
				ID:               oldID,
				Username:         user.Username,
				RefreshTokenHash: oldHash,
				Now:              now,
				NewSession: func() (SessionReplacement, error) {
					return SessionReplacement{
						ID:               oldID,
						RefreshTokenHash: uuid.NewString(),
						ExpiresAt:        time.Now().Add(time.Hour),
					}, nil
				},
			})
			if !errors.Is(err, ErrInvalidSession) {
				t.Fatalf("want ErrInvalidSession, got %v", err)
			}
		})
	}
}

func TestRotateSessionTx_NonExistentSession(t *testing.T) {
	user := createTestUser(t)
	nonExistentID := uuid.New() // Fresh UUID, never inserted

	callbackCalled := false
	_, err := testStore.RotateSessionTx(t.Context(), RotateSessionTxParams{
		ID:               nonExistentID,
		Username:         user.Username,
		RefreshTokenHash: "any-hash",
		Now:              time.Now(),
		NewSession: func() (SessionReplacement, error) {
			callbackCalled = true
			t.Fatal("NewSession callback should not be called for nonexistent session")
			return SessionReplacement{}, nil
		},
	})
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession, got %v", err)
	}
	if callbackCalled {
		t.Fatal("NewSession callback was called but should not have been")
	}
}

func TestRotateSessionTx_TokenHashMismatch(t *testing.T) {
	user := createTestUser(t)
	oldID := uuid.New()
	const oldHash = "old-refresh-hash"

	_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
		ID:           oldID,
		Username:     user.Username,
		RefreshToken: oldHash,
		UserAgent:    "test-agent",
		ClientIp:     "127.0.0.1",
		IsBlocked:    false,
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = testStore.RotateSessionTx(t.Context(), RotateSessionTxParams{
		ID:               oldID,
		Username:         user.Username,
		RefreshTokenHash: "wrong-hash", // Mismatch
		Now:              time.Now(),
		NewSession: func() (SessionReplacement, error) {
			return SessionReplacement{
				ID:               uuid.New(),
				RefreshTokenHash: uuid.NewString(),
				ExpiresAt:        time.Now().Add(time.Hour),
			}, nil
		},
	})
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession, got %v", err)
	}
}

func TestRotateSessionTx_UsernameMismatch(t *testing.T) {
	user := createTestUser(t)
	oldID := uuid.New()
	const oldHash = "old-refresh-hash"

	_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
		ID:           oldID,
		Username:     user.Username,
		RefreshToken: oldHash,
		UserAgent:    "test-agent",
		ClientIp:     "127.0.0.1",
		IsBlocked:    false,
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = testStore.RotateSessionTx(t.Context(), RotateSessionTxParams{
		ID:               oldID,
		Username:         "wrong-username", // Mismatch
		RefreshTokenHash: oldHash,
		Now:              time.Now(),
		NewSession: func() (SessionReplacement, error) {
			return SessionReplacement{
				ID:               uuid.New(),
				RefreshTokenHash: uuid.NewString(),
				ExpiresAt:        time.Now().Add(time.Hour),
			}, nil
		},
	})
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession, got %v", err)
	}
}

func TestRotateSessionTx_ConcurrentReuse(t *testing.T) {
	user := createTestUser(t)
	oldID := uuid.New()
	const oldHash = "old-refresh-hash"

	_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
		ID:           oldID,
		Username:     user.Username,
		RefreshToken: oldHash,
		UserAgent:    "test-agent",
		ClientIp:     "127.0.0.1",
		IsBlocked:    false,
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := testStore.RotateSessionTx(t.Context(), RotateSessionTxParams{
				ID:               oldID,
				Username:         user.Username,
				RefreshTokenHash: oldHash,
				Now:              time.Now(),
				NewSession: func() (SessionReplacement, error) {
					return SessionReplacement{
						ID:               oldID,
						RefreshTokenHash: uuid.NewString(),
						ExpiresAt:        time.Now().Add(time.Hour),
					}, nil
				},
			})
			results <- err
		}()
	}

	var successes, invalid int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrInvalidSession):
			invalid++
		default:
			t.Fatalf("unexpected rotation error: %v", err)
		}
	}
	if successes != 1 || invalid != 1 {
		t.Fatalf("successes=%d invalid=%d, want 1 and 1", successes, invalid)
	}
}

func TestRotateSessionTx_LogoutInterleavingsStableID(t *testing.T) {
	t.Run("logout wins before renew", func(t *testing.T) {
		user := createTestUser(t)
		sessionID := uuid.New()
		const oldHash = "old-refresh-hash"

		_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
			ID:           sessionID,
			Username:     user.Username,
			RefreshToken: oldHash,
			UserAgent:    "test-agent",
			ClientIp:     "127.0.0.1",
			IsBlocked:    false,
			ExpiresAt:    time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := testStore.BlockSession(t.Context(), sessionID); err != nil {
			t.Fatalf("block session: %v", err)
		}
		_, err = testStore.RotateSessionTx(t.Context(), RotateSessionTxParams{
			ID:               sessionID,
			Username:         user.Username,
			RefreshTokenHash: oldHash,
			Now:              time.Now(),
			NewSession: func() (SessionReplacement, error) {
				return SessionReplacement{
					ID:               sessionID,
					RefreshTokenHash: uuid.NewString(),
					ExpiresAt:        time.Now().Add(time.Hour),
				}, nil
			},
		})
		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("want ErrInvalidSession after logout wins, got %v", err)
		}
		got, err := testStore.GetSession(t.Context(), sessionID)
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		if !got.IsBlocked || got.RefreshToken != oldHash {
			t.Fatalf("final session = blocked:%v hash:%q, want blocked old hash", got.IsBlocked, got.RefreshToken)
		}
	})

	t.Run("renew wins before logout", func(t *testing.T) {
		user := createTestUser(t)
		sessionID := uuid.New()
		const oldHash = "old-refresh-hash"
		newHash := uuid.NewString()

		_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
			ID:           sessionID,
			Username:     user.Username,
			RefreshToken: oldHash,
			UserAgent:    "test-agent",
			ClientIp:     "127.0.0.1",
			IsBlocked:    false,
			ExpiresAt:    time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}

		rotated, err := testStore.RotateSessionTx(t.Context(), RotateSessionTxParams{
			ID:               sessionID,
			Username:         user.Username,
			RefreshTokenHash: oldHash,
			Now:              time.Now(),
			NewSession: func() (SessionReplacement, error) {
				return SessionReplacement{
					ID:               sessionID,
					RefreshTokenHash: newHash,
					ExpiresAt:        time.Now().Add(time.Hour),
				}, nil
			},
		})
		if err != nil {
			t.Fatalf("rotate session: %v", err)
		}
		if rotated.ID != sessionID || rotated.RefreshToken != newHash {
			t.Fatalf("rotated session = id:%s hash:%q, want stable id %s and new hash %q", rotated.ID, rotated.RefreshToken, sessionID, newHash)
		}

		if _, err := testStore.BlockSession(t.Context(), sessionID); err != nil {
			t.Fatalf("block rotated session by stable ID: %v", err)
		}
		got, err := testStore.GetSession(t.Context(), sessionID)
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		if !got.IsBlocked || got.RefreshToken != newHash {
			t.Fatalf("final session = blocked:%v hash:%q, want blocked rotated hash %q", got.IsBlocked, got.RefreshToken, newHash)
		}
	})
}

func TestValidateAccessSession_Valid(t *testing.T) {
	user := createTestUser(t)
	sessionID := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
		ID:           sessionID,
		Username:     user.Username,
		RefreshToken: "refresh-hash",
		UserAgent:    "test-agent",
		ClientIp:     "127.0.0.1",
		IsBlocked:    false,
		ExpiresAt:    now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := testStore.ValidateAccessSession(t.Context(), sessionID, user.Username, now); err != nil {
		t.Fatalf("ValidateAccessSession failed: %v", err)
	}
}

func TestValidateAccessSession_Missing(t *testing.T) {
	err := testStore.ValidateAccessSession(t.Context(), uuid.New(), "alice", time.Now())
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession, got %v", err)
	}
}

func TestValidateAccessSession_Blocked(t *testing.T) {
	user := createTestUser(t)
	sessionID := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
		ID:           sessionID,
		Username:     user.Username,
		RefreshToken: "refresh-hash",
		UserAgent:    "test-agent",
		ClientIp:     "127.0.0.1",
		IsBlocked:    true,
		ExpiresAt:    now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	err = testStore.ValidateAccessSession(t.Context(), sessionID, user.Username, now)
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession, got %v", err)
	}
}

func TestValidateAccessSession_Expired(t *testing.T) {
	user := createTestUser(t)
	sessionID := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
		ID:           sessionID,
		Username:     user.Username,
		RefreshToken: "refresh-hash",
		UserAgent:    "test-agent",
		ClientIp:     "127.0.0.1",
		IsBlocked:    false,
		ExpiresAt:    now,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = testStore.ValidateAccessSession(t.Context(), sessionID, user.Username, now)
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession, got %v", err)
	}
}

func TestValidateAccessSession_UsernameMismatch(t *testing.T) {
	user := createTestUser(t)
	sessionID := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
		ID:           sessionID,
		Username:     user.Username,
		RefreshToken: "refresh-hash",
		UserAgent:    "test-agent",
		ClientIp:     "127.0.0.1",
		IsBlocked:    false,
		ExpiresAt:    now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	err = testStore.ValidateAccessSession(t.Context(), sessionID, "bob", now)
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession, got %v", err)
	}
}
