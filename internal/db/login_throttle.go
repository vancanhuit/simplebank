package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

const (
	loginThrottleWindow       = 15 * time.Minute
	loginThrottleBaseCooldown = 30 * time.Second
	loginThrottleMaxCooldown  = 15 * time.Minute
	loginThrottleRetention    = 30 * time.Minute
	accountAttemptThreshold   = int32(5)
	clientAttemptThreshold    = int32(20)
	loginThrottleScopeAccount = "account"
	loginThrottleScopeClient  = "client"
)

var errLoginAttemptBlocked = errors.New("login attempt blocked")

type LoginAttemptAdmission struct {
	RetryAfter time.Duration
}

func (s *SQLStore) ReserveLoginAttempt(
	ctx context.Context,
	username string,
	clientIP string,
	now time.Time,
) (LoginAttemptAdmission, error) {
	accountKeyHash := loginThrottleHash(
		loginThrottleScopeAccount,
		normalizeLoginUsername(username),
	)
	clientKeyHash := loginThrottleHash(
		loginThrottleScopeClient,
		normalizeClientIP(clientIP),
	)

	var admission LoginAttemptAdmission
	err := s.execTx(ctx, func(q *sqlcdb.Queries) error {
		if _, err := q.DeleteExpiredLoginThrottles(ctx, now); err != nil {
			return ClassifyError(err)
		}
		if err := q.InitializeLoginThrottlePair(ctx, sqlcdb.InitializeLoginThrottlePairParams{
			AccountScope:     loginThrottleScopeAccount,
			AccountKeyHash:   accountKeyHash,
			Now:              now,
			RetentionSeconds: int32(loginThrottleRetention / time.Second),
			ClientScope:      loginThrottleScopeClient,
			ClientKeyHash:    clientKeyHash,
		}); err != nil {
			return ClassifyError(err)
		}

		throttles, err := q.GetLoginThrottlePairForUpdate(
			ctx,
			sqlcdb.GetLoginThrottlePairForUpdateParams{
				AccountScope:   loginThrottleScopeAccount,
				AccountKeyHash: accountKeyHash,
				ClientScope:    loginThrottleScopeClient,
				ClientKeyHash:  clientKeyHash,
			},
		)
		if err != nil {
			return ClassifyError(err)
		}
		if len(throttles) != 2 {
			return fmt.Errorf("lock login throttle pair: got %d rows, want 2", len(throttles))
		}

		for _, throttle := range throttles {
			retryAfter := retryAfterFromThrottle(throttle, now)
			if retryAfter > admission.RetryAfter {
				admission.RetryAfter = retryAfter
			}
		}
		if admission.RetryAfter > 0 {
			return errLoginAttemptBlocked
		}

		if err := advanceLoginThrottleAttempt(
			ctx,
			q,
			loginThrottleScopeAccount,
			accountKeyHash,
			now,
			accountAttemptThreshold,
		); err != nil {
			return err
		}
		return advanceLoginThrottleAttempt(
			ctx,
			q,
			loginThrottleScopeClient,
			clientKeyHash,
			now,
			clientAttemptThreshold,
		)
	})
	if errors.Is(err, errLoginAttemptBlocked) {
		return admission, nil
	}
	return admission, err
}

func (s *SQLStore) ClearLoginAccountThrottle(
	ctx context.Context,
	username string,
) error {
	return ClassifyError(s.DeleteLoginThrottle(ctx, sqlcdb.DeleteLoginThrottleParams{
		Scope:   loginThrottleScopeAccount,
		KeyHash: loginThrottleHash(loginThrottleScopeAccount, normalizeLoginUsername(username)),
	}))
}

func advanceLoginThrottleAttempt(
	ctx context.Context,
	q *sqlcdb.Queries,
	scope string,
	keyHash string,
	now time.Time,
	threshold int32,
) error {
	throttle, err := q.AdvanceLoginThrottleAttempt(ctx, sqlcdb.AdvanceLoginThrottleAttemptParams{
		Now:              now,
		WindowSeconds:    int32(loginThrottleWindow / time.Second),
		RetentionSeconds: int32(loginThrottleRetention / time.Second),
		Scope:            scope,
		KeyHash:          keyHash,
	})
	if err != nil {
		return ClassifyError(err)
	}

	cooldown := loginCooldown(throttle.AttemptCount, threshold)
	if cooldown == 0 {
		return nil
	}

	_, err = q.SetLoginThrottleBlockedUntil(ctx, sqlcdb.SetLoginThrottleBlockedUntilParams{
		BlockedUntil: now.Add(cooldown),
		Scope:        scope,
		KeyHash:      keyHash,
	})
	return ClassifyError(err)
}

func retryAfterFromThrottle(throttle sqlcdb.LoginThrottle, now time.Time) time.Duration {
	if !throttle.BlockedUntil.Valid {
		return 0
	}
	retryAfter := throttle.BlockedUntil.Time.Sub(now)
	if retryAfter <= 0 {
		return 0
	}
	return retryAfter
}

func loginThrottleHash(scope, raw string) string {
	sum := sha256.Sum256([]byte(scope + "\x00" + raw))
	return hex.EncodeToString(sum[:])
}

func normalizeLoginUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func normalizeClientIP(clientIP string) string {
	trimmed := strings.TrimSpace(clientIP)
	if ip := net.ParseIP(trimmed); ip != nil {
		return ip.String()
	}
	return trimmed
}

func loginCooldown(attempts, threshold int32) time.Duration {
	if attempts < threshold {
		return 0
	}

	exponent := attempts - threshold
	if exponent > 10 {
		exponent = 10
	}

	cooldown := loginThrottleBaseCooldown * time.Duration(1<<exponent)
	if cooldown > loginThrottleMaxCooldown {
		return loginThrottleMaxCooldown
	}
	return cooldown
}
