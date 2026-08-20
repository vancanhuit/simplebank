package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	accountFailureThreshold   = int32(5)
	clientFailureThreshold    = int32(20)
	loginThrottleScopeAccount = "account"
	loginThrottleScopeClient  = "client"
)

type LoginThrottleDecision struct {
	RetryAfter time.Duration
}

func (s *SQLStore) CheckLoginThrottle(
	ctx context.Context,
	username string,
	clientIP string,
	now time.Time,
) (LoginThrottleDecision, error) {
	accountRetryAfter, err := s.loginThrottleRetryAfter(
		ctx,
		loginThrottleScopeAccount,
		loginThrottleHash(loginThrottleScopeAccount, normalizeLoginUsername(username)),
		now,
	)
	if err != nil {
		return LoginThrottleDecision{}, err
	}

	clientRetryAfter, err := s.loginThrottleRetryAfter(
		ctx,
		loginThrottleScopeClient,
		loginThrottleHash(loginThrottleScopeClient, normalizeClientIP(clientIP)),
		now,
	)
	if err != nil {
		return LoginThrottleDecision{}, err
	}

	if clientRetryAfter > accountRetryAfter {
		accountRetryAfter = clientRetryAfter
	}
	return LoginThrottleDecision{RetryAfter: accountRetryAfter}, nil
}

func (s *SQLStore) RecordLoginFailure(
	ctx context.Context,
	username string,
	clientIP string,
	now time.Time,
) (LoginThrottleDecision, error) {
	normalizedUsername := normalizeLoginUsername(username)
	normalizedClientIP := normalizeClientIP(clientIP)
	accountKeyHash := loginThrottleHash(loginThrottleScopeAccount, normalizedUsername)
	clientKeyHash := loginThrottleHash(loginThrottleScopeClient, normalizedClientIP)

	var decision LoginThrottleDecision
	err := s.execTx(ctx, func(q *sqlcdb.Queries) error {
		if _, err := q.DeleteExpiredLoginThrottles(ctx, now); err != nil {
			return ClassifyError(err)
		}

		accountRetryAfter, err := recordLoginThrottleFailure(
			ctx,
			q,
			loginThrottleScopeAccount,
			accountKeyHash,
			now,
			accountFailureThreshold,
		)
		if err != nil {
			return err
		}

		clientRetryAfter, err := recordLoginThrottleFailure(
			ctx,
			q,
			loginThrottleScopeClient,
			clientKeyHash,
			now,
			clientFailureThreshold,
		)
		if err != nil {
			return err
		}

		if clientRetryAfter > accountRetryAfter {
			accountRetryAfter = clientRetryAfter
		}
		decision.RetryAfter = accountRetryAfter
		return nil
	})
	return decision, err
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

func recordLoginThrottleFailure(
	ctx context.Context,
	q *sqlcdb.Queries,
	scope string,
	keyHash string,
	now time.Time,
	threshold int32,
) (time.Duration, error) {
	throttle, err := q.IncrementLoginThrottle(ctx, sqlcdb.IncrementLoginThrottleParams{
		Scope:            scope,
		KeyHash:          keyHash,
		Now:              now,
		RetentionSeconds: int32(loginThrottleRetention / time.Second),
		WindowSeconds:    int32(loginThrottleWindow / time.Second),
	})
	if err != nil {
		return 0, ClassifyError(err)
	}

	cooldown := loginCooldown(throttle.FailureCount, threshold)
	if cooldown == 0 {
		return 0, nil
	}

	blockedUntil := now.Add(cooldown)
	if _, err := q.SetLoginThrottleBlockedUntil(ctx, sqlcdb.SetLoginThrottleBlockedUntilParams{
		BlockedUntil: blockedUntil,
		Scope:        scope,
		KeyHash:      keyHash,
	}); err != nil {
		return 0, ClassifyError(err)
	}
	return cooldown, nil
}

func (s *SQLStore) loginThrottleRetryAfter(
	ctx context.Context,
	scope string,
	keyHash string,
	now time.Time,
) (time.Duration, error) {
	throttle, err := s.GetLoginThrottle(ctx, sqlcdb.GetLoginThrottleParams{
		Scope:   scope,
		KeyHash: keyHash,
	})
	if err != nil {
		classified := ClassifyError(err)
		if errors.Is(classified, ErrRecordNotFound) {
			return 0, nil
		}
		return 0, classified
	}
	return retryAfterFromThrottle(throttle, now), nil
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

func loginCooldown(failures, threshold int32) time.Duration {
	if failures < threshold {
		return 0
	}

	exponent := failures - threshold
	if exponent > 10 {
		exponent = 10
	}

	cooldown := loginThrottleBaseCooldown * time.Duration(1<<exponent)
	if cooldown > loginThrottleMaxCooldown {
		return loginThrottleMaxCooldown
	}
	return cooldown
}
