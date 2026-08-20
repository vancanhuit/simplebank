//go:build integration

package store

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

func TestLoginThrottle_AccountBlocksAtFifthFailure(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("account")
	clientIP := throttleTestIP()

	for i := range 4 {
		decision, err := testStore.RecordLoginFailure(t.Context(), username, clientIP, now)
		if err != nil {
			t.Fatalf("failure %d: %v", i+1, err)
		}
		if decision.RetryAfter != 0 {
			t.Fatalf("failure %d retry_after = %s, want 0", i+1, decision.RetryAfter)
		}
	}

	decision, err := testStore.RecordLoginFailure(t.Context(), username, clientIP, now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.RetryAfter != 30*time.Second {
		t.Fatalf("fifth failure retry_after = %s, want %s", decision.RetryAfter, 30*time.Second)
	}

	check, err := testStore.CheckLoginThrottle(t.Context(), lowerTrim(username), lowerTrim(clientIP), now)
	if err != nil {
		t.Fatal(err)
	}
	if check.RetryAfter != 30*time.Second {
		t.Fatalf("check retry_after = %s, want %s", check.RetryAfter, 30*time.Second)
	}
}

func TestLoginThrottle_ClientBlocksAtTwentiethFailure(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	clientIP := throttleTestIP()

	for i := range 19 {
		decision, err := testStore.RecordLoginFailure(
			t.Context(),
			throttleTestUsername(fmt.Sprintf("client-%02d", i)),
			clientIP,
			now,
		)
		if err != nil {
			t.Fatalf("failure %d: %v", i+1, err)
		}
		if decision.RetryAfter != 0 {
			t.Fatalf("failure %d retry_after = %s, want 0", i+1, decision.RetryAfter)
		}
	}

	decision, err := testStore.RecordLoginFailure(
		t.Context(),
		throttleTestUsername("client-threshold"),
		clientIP,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.RetryAfter != 30*time.Second {
		t.Fatalf("twentieth failure retry_after = %s, want %s", decision.RetryAfter, 30*time.Second)
	}

	check, err := testStore.CheckLoginThrottle(t.Context(), throttleTestUsername("other-user"), clientIP, now)
	if err != nil {
		t.Fatal(err)
	}
	if check.RetryAfter != 30*time.Second {
		t.Fatalf("client throttle retry_after = %s, want %s", check.RetryAfter, 30*time.Second)
	}
}

func TestLoginThrottle_CooldownDoublesAndCaps(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("cooldown")
	clientIP := throttleTestIP()
	wantByFailure := map[int]time.Duration{
		5:  30 * time.Second,
		6:  60 * time.Second,
		7:  120 * time.Second,
		8:  240 * time.Second,
		9:  480 * time.Second,
		10: 15 * time.Minute,
		11: 15 * time.Minute,
	}

	for i := range 11 {
		decision, err := testStore.RecordLoginFailure(t.Context(), username, clientIP, now)
		if err != nil {
			t.Fatalf("failure %d: %v", i+1, err)
		}
		if want, ok := wantByFailure[i+1]; ok && decision.RetryAfter != want {
			t.Fatalf("failure %d retry_after = %s, want %s", i+1, decision.RetryAfter, want)
		}
	}
}

func TestLoginThrottle_WindowResetStartsAtOne(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("window")
	clientIP := throttleTestIP()

	for i := range 4 {
		decision, err := testStore.RecordLoginFailure(t.Context(), username, clientIP, start)
		if err != nil {
			t.Fatalf("initial failure %d: %v", i+1, err)
		}
		if decision.RetryAfter != 0 {
			t.Fatalf("initial failure %d retry_after = %s, want 0", i+1, decision.RetryAfter)
		}
	}

	resetAt := start.Add(15 * time.Minute)
	decision, err := testStore.RecordLoginFailure(t.Context(), username, clientIP, resetAt)
	if err != nil {
		t.Fatal(err)
	}
	if decision.RetryAfter != 0 {
		t.Fatalf("reset failure retry_after = %s, want 0", decision.RetryAfter)
	}

	for i := range 3 {
		decision, err = testStore.RecordLoginFailure(t.Context(), username, clientIP, resetAt)
		if err != nil {
			t.Fatalf("post-reset failure %d: %v", i+2, err)
		}
		if decision.RetryAfter != 0 {
			t.Fatalf("post-reset failure %d retry_after = %s, want 0", i+2, decision.RetryAfter)
		}
	}

	decision, err = testStore.RecordLoginFailure(t.Context(), username, clientIP, resetAt)
	if err != nil {
		t.Fatal(err)
	}
	if decision.RetryAfter != 30*time.Second {
		t.Fatalf("fifth post-reset failure retry_after = %s, want %s", decision.RetryAfter, 30*time.Second)
	}
}

func TestLoginThrottle_ClearAccountPreservesClient(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("clear-account")
	clientIP := throttleTestIP()

	if _, err := testStore.RecordLoginFailure(t.Context(), username, clientIP, now); err != nil {
		t.Fatal(err)
	}
	for i := range 19 {
		if _, err := testStore.RecordLoginFailure(
			t.Context(),
			throttleTestUsername(fmt.Sprintf("clear-client-%02d", i)),
			clientIP,
			now,
		); err != nil {
			t.Fatalf("client failure %d: %v", i+2, err)
		}
	}

	if err := testStore.ClearLoginAccountThrottle(t.Context(), username); err != nil {
		t.Fatal(err)
	}

	clientBlocked, err := testStore.CheckLoginThrottle(t.Context(), lowerTrim(username), clientIP, now)
	if err != nil {
		t.Fatal(err)
	}
	if clientBlocked.RetryAfter != 30*time.Second {
		t.Fatalf("same IP retry_after = %s, want %s", clientBlocked.RetryAfter, 30*time.Second)
	}

	accountCleared, err := testStore.CheckLoginThrottle(t.Context(), lowerTrim(username), throttleTestIP(), now)
	if err != nil {
		t.Fatal(err)
	}
	if accountCleared.RetryAfter != 0 {
		t.Fatalf("different IP retry_after = %s, want 0", accountCleared.RetryAfter)
	}
}

func TestLoginThrottle_IsSharedAcrossStoreInstances(t *testing.T) {
	otherStore := New(testPool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("shared")
	clientIP := "203.0.113.10"

	for range 5 {
		if _, err := testStore.RecordLoginFailure(
			t.Context(), username, clientIP, now,
		); err != nil {
			t.Fatal(err)
		}
	}

	decision, err := otherStore.CheckLoginThrottle(
		t.Context(), username, clientIP, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.RetryAfter <= 0 {
		t.Fatal("second store instance must observe account cooldown")
	}
}

func TestLoginThrottle_CheckUsesSingleSnapshot(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("single-snapshot")
	clientIP := throttleTestIP()
	baseStore := New(testPool)

	for i := range 4 {
		decision, err := baseStore.RecordLoginFailure(t.Context(), username, clientIP, now)
		if err != nil {
			t.Fatalf("seed failure %d: %v", i+1, err)
		}
		if decision.RetryAfter != 0 {
			t.Fatalf("seed failure %d retry_after = %s, want 0", i+1, decision.RetryAfter)
		}
	}

	hookedStore := &SQLStore{
		Queries: sqlcdb.New(&loginThrottleSnapshotHookDB{
			inner: testPool,
			runHook: func(ctx context.Context) error {
				_, err := baseStore.RecordLoginFailure(ctx, username, clientIP, now)
				return err
			},
		}),
		connPool: testPool,
	}

	decision, err := hookedStore.CheckLoginThrottle(t.Context(), username, clientIP, now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.RetryAfter != 30*time.Second {
		t.Fatalf("check retry_after = %s, want %s", decision.RetryAfter, 30*time.Second)
	}

	observed, err := baseStore.CheckLoginThrottle(t.Context(), username, clientIP, now)
	if err != nil {
		t.Fatal(err)
	}
	if observed.RetryAfter != 30*time.Second {
		t.Fatalf("persisted retry_after = %s, want %s", observed.RetryAfter, 30*time.Second)
	}
}

func TestLoginThrottle_ExpiredRowsAreDeleted(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Microsecond)
	oldUsername := throttleTestUsername("expired-old")
	oldIP := throttleTestIP()
	oldAccountHash := loginThrottleHash(loginThrottleScopeAccount, normalizeLoginUsername(oldUsername))
	oldClientHash := loginThrottleHash(loginThrottleScopeClient, normalizeClientIP(oldIP))

	if _, err := testStore.RecordLoginFailure(t.Context(), oldUsername, oldIP, start); err != nil {
		t.Fatal(err)
	}
	if got := loginThrottleKeyCount(t, oldAccountHash, oldClientHash); got != 2 {
		t.Fatalf("initial expired-key rows = %d, want 2", got)
	}

	newUsername := throttleTestUsername("expired-new")
	newIP := throttleTestIP()
	next := start.Add(30*time.Minute + time.Microsecond)
	if _, err := testStore.RecordLoginFailure(
		t.Context(),
		newUsername,
		newIP,
		next,
	); err != nil {
		t.Fatal(err)
	}
	if got := loginThrottleKeyCount(t, oldAccountHash, oldClientHash); got != 0 {
		t.Fatalf("expired-key rows after cleanup = %d, want 0", got)
	}
	newAccountHash := loginThrottleHash(loginThrottleScopeAccount, normalizeLoginUsername(newUsername))
	newClientHash := loginThrottleHash(loginThrottleScopeClient, normalizeClientIP(newIP))
	if got := loginThrottleKeyCount(t, newAccountHash, newClientHash); got != 2 {
		t.Fatalf("new-key rows after cleanup = %d, want 2", got)
	}

	decision, err := testStore.CheckLoginThrottle(t.Context(), oldUsername, oldIP, next)
	if err != nil {
		t.Fatal(err)
	}
	if decision.RetryAfter != 0 {
		t.Fatalf("expired key retry_after = %s, want 0", decision.RetryAfter)
	}
}

func throttleTestUsername(prefix string) string {
	return fmt.Sprintf("  %s-%s  ", prefix, uuid.NewString())
}

func throttleTestIP() string {
	id := uuid.New()
	return fmt.Sprintf(" 203.0.%d.%d ", id[0], id[1])
}

func lowerTrim(value string) string {
	return normalizeLoginUsername(value)
}

func loginThrottleKeyCount(t *testing.T, keyHashes ...string) int {
	t.Helper()

	var count int
	if err := testPool.QueryRow(
		t.Context(),
		"SELECT count(*) FROM login_throttles WHERE key_hash = ANY($1::text[])",
		keyHashes,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

type loginThrottleSnapshotHookDB struct {
	inner   sqlcdb.DBTX
	runHook func(context.Context) error
	fired   atomic.Bool
}

func (db *loginThrottleSnapshotHookDB) Exec(
	ctx context.Context,
	sql string,
	args ...interface{},
) (pgconn.CommandTag, error) {
	return db.inner.Exec(ctx, sql, args...)
}

func (db *loginThrottleSnapshotHookDB) Query(
	ctx context.Context,
	sql string,
	args ...interface{},
) (pgx.Rows, error) {
	if strings.Contains(sql, "-- name: GetLoginThrottleSnapshot :many") {
		if err := db.fire(ctx); err != nil {
			return nil, err
		}
	}
	return db.inner.Query(ctx, sql, args...)
}

func (db *loginThrottleSnapshotHookDB) QueryRow(
	ctx context.Context,
	sql string,
	args ...interface{},
) pgx.Row {
	row := db.inner.QueryRow(ctx, sql, args...)
	if strings.Contains(sql, "-- name: GetLoginThrottle :one") &&
		len(args) > 0 &&
		args[0] == loginThrottleScopeAccount {
		return loginThrottleSnapshotHookRow{
			Row: row,
			hook: func() error {
				return db.fire(ctx)
			},
		}
	}
	return row
}

func (db *loginThrottleSnapshotHookDB) fire(ctx context.Context) error {
	if !db.fired.CompareAndSwap(false, true) {
		return nil
	}
	return db.runHook(ctx)
}

type loginThrottleSnapshotHookRow struct {
	pgx.Row
	hook func() error
}

func (r loginThrottleSnapshotHookRow) Scan(dest ...interface{}) error {
	if err := r.Row.Scan(dest...); err != nil {
		return err
	}
	if r.hook != nil {
		return r.hook()
	}
	return nil
}
