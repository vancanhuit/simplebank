//go:build integration

package store

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLoginAttemptReservation_AccountThresholdAdmitsFifthAttempt(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("account")
	clientIP := throttleTestIP()

	for attempt := range int(accountAttemptThreshold) {
		admission, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, now)
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt+1, err)
		}
		if admission.RetryAfter != 0 {
			t.Fatalf("attempt %d retry_after = %s, want admitted", attempt+1, admission.RetryAfter)
		}
	}

	denied, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, now)
	if err != nil {
		t.Fatal(err)
	}
	if denied.RetryAfter != 30*time.Second {
		t.Fatalf("sixth attempt retry_after = %s, want %s", denied.RetryAfter, 30*time.Second)
	}

	accountHash := loginThrottleHash(loginThrottleScopeAccount, normalizeLoginUsername(username))
	if got := loginThrottleAttemptCount(t, loginThrottleScopeAccount, accountHash); got != accountAttemptThreshold {
		t.Fatalf("account attempt_count = %d, want %d", got, accountAttemptThreshold)
	}
}

func TestLoginAttemptReservation_ClientThresholdAdmitsTwentiethAttempt(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	clientIP := throttleTestIP()

	for attempt := range int(clientAttemptThreshold) {
		admission, err := testStore.ReserveLoginAttempt(
			t.Context(),
			throttleTestUsername(fmt.Sprintf("client-%02d", attempt)),
			clientIP,
			now,
		)
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt+1, err)
		}
		if admission.RetryAfter != 0 {
			t.Fatalf("attempt %d retry_after = %s, want admitted", attempt+1, admission.RetryAfter)
		}
	}

	denied, err := testStore.ReserveLoginAttempt(
		t.Context(),
		throttleTestUsername("client-denied"),
		clientIP,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if denied.RetryAfter != 30*time.Second {
		t.Fatalf("twenty-first attempt retry_after = %s, want %s", denied.RetryAfter, 30*time.Second)
	}

	clientHash := loginThrottleHash(loginThrottleScopeClient, normalizeClientIP(clientIP))
	if got := loginThrottleAttemptCount(t, loginThrottleScopeClient, clientHash); got != clientAttemptThreshold {
		t.Fatalf("client attempt_count = %d, want %d", got, clientAttemptThreshold)
	}
}

func TestLoginAttemptReservation_CooldownDoublesAfterExpiryAndCaps(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("cooldown")
	clientIP := throttleTestIP()

	for attempt := range int(accountAttemptThreshold - 1) {
		admission, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, now)
		if err != nil {
			t.Fatalf("seed attempt %d: %v", attempt+1, err)
		}
		if admission.RetryAfter != 0 {
			t.Fatalf("seed attempt %d retry_after = %s, want admitted", attempt+1, admission.RetryAfter)
		}
	}

	for attempt, want := range []time.Duration{
		30 * time.Second,
		60 * time.Second,
		120 * time.Second,
		240 * time.Second,
		480 * time.Second,
		15 * time.Minute,
		15 * time.Minute,
	} {
		admission, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, now)
		if err != nil {
			t.Fatalf("cooldown attempt %d: %v", attempt+int(accountAttemptThreshold), err)
		}
		if admission.RetryAfter != 0 {
			t.Fatalf("cooldown attempt %d was not admitted: %s", attempt+int(accountAttemptThreshold), admission.RetryAfter)
		}

		denied, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, now)
		if err != nil {
			t.Fatalf("cooldown check %d: %v", attempt+1, err)
		}
		if denied.RetryAfter != want {
			t.Fatalf("cooldown %d retry_after = %s, want %s", attempt+1, denied.RetryAfter, want)
		}
		now = now.Add(want)
	}
}

func TestLoginAttemptReservation_InactivityWindowResetsCount(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("window")
	clientIP := throttleTestIP()

	for attempt := range int(accountAttemptThreshold - 1) {
		admission, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, start)
		if err != nil {
			t.Fatalf("initial attempt %d: %v", attempt+1, err)
		}
		if admission.RetryAfter != 0 {
			t.Fatalf("initial attempt %d retry_after = %s, want admitted", attempt+1, admission.RetryAfter)
		}
	}

	resetAt := start.Add(loginThrottleWindow + time.Microsecond)
	for attempt := range int(accountAttemptThreshold) {
		admission, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, resetAt)
		if err != nil {
			t.Fatalf("post-reset attempt %d: %v", attempt+1, err)
		}
		if admission.RetryAfter != 0 {
			t.Fatalf("post-reset attempt %d retry_after = %s, want admitted", attempt+1, admission.RetryAfter)
		}
	}

	denied, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, resetAt)
	if err != nil {
		t.Fatal(err)
	}
	if denied.RetryAfter != 30*time.Second {
		t.Fatalf("post-reset denied retry_after = %s, want %s", denied.RetryAfter, 30*time.Second)
	}
}

func TestLoginAttemptReservation_ClearAccountPreservesClient(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("clear-account")
	clientIP := throttleTestIP()

	if _, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, now); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt < int(clientAttemptThreshold); attempt++ {
		if _, err := testStore.ReserveLoginAttempt(
			t.Context(),
			throttleTestUsername(fmt.Sprintf("clear-client-%02d", attempt)),
			clientIP,
			now,
		); err != nil {
			t.Fatalf("client attempt %d: %v", attempt+1, err)
		}
	}

	if err := testStore.ClearLoginAccountThrottle(t.Context(), username); err != nil {
		t.Fatal(err)
	}

	clientDenied, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, now)
	if err != nil {
		t.Fatal(err)
	}
	if clientDenied.RetryAfter != 30*time.Second {
		t.Fatalf("same IP retry_after = %s, want %s", clientDenied.RetryAfter, 30*time.Second)
	}

	accountAdmitted, err := testStore.ReserveLoginAttempt(t.Context(), username, throttleTestIP(), now)
	if err != nil {
		t.Fatal(err)
	}
	if accountAdmitted.RetryAfter != 0 {
		t.Fatalf("cleared account retry_after = %s, want admitted", accountAdmitted.RetryAfter)
	}
}

func TestLoginAttemptReservation_IsSharedAcrossStoreInstances(t *testing.T) {
	otherStore := New(testPool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("shared")
	clientIP := throttleTestIP()

	for range int(accountAttemptThreshold) {
		if _, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, now); err != nil {
			t.Fatal(err)
		}
	}

	denied, err := otherStore.ReserveLoginAttempt(t.Context(), username, clientIP, now)
	if err != nil {
		t.Fatal(err)
	}
	if denied.RetryAfter <= 0 {
		t.Fatal("second store instance must observe account cooldown")
	}
}

func TestLoginAttemptReservation_ConcurrentAccountAdmissionsAreBounded(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	username := throttleTestUsername("concurrent-account")
	clientIP := throttleTestIP()

	admitted, denied := runConcurrentReservations(t, 30, func(int) (string, string) {
		return username, clientIP
	}, now)

	if admitted != int(accountAttemptThreshold) {
		t.Fatalf("admitted account attempts = %d, want %d", admitted, accountAttemptThreshold)
	}
	if denied != 30-int(accountAttemptThreshold) {
		t.Fatalf("denied account attempts = %d, want %d", denied, 30-int(accountAttemptThreshold))
	}
}

func TestLoginAttemptReservation_ConcurrentClientAdmissionsAreBounded(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	clientIP := throttleTestIP()
	usernames := make([]string, 40)
	for index := range usernames {
		usernames[index] = throttleTestUsername(fmt.Sprintf("concurrent-client-%02d", index))
	}

	admitted, denied := runConcurrentReservations(t, len(usernames), func(index int) (string, string) {
		return usernames[index], clientIP
	}, now)

	if admitted != int(clientAttemptThreshold) {
		t.Fatalf("admitted client attempts = %d, want %d", admitted, clientAttemptThreshold)
	}
	if denied != len(usernames)-int(clientAttemptThreshold) {
		t.Fatalf("denied client attempts = %d, want %d", denied, len(usernames)-int(clientAttemptThreshold))
	}
}

func TestLoginAttemptReservation_ExpiredRowsAreDeleted(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Microsecond)
	oldUsername := throttleTestUsername("expired-old")
	oldIP := throttleTestIP()
	oldAccountHash := loginThrottleHash(loginThrottleScopeAccount, normalizeLoginUsername(oldUsername))
	oldClientHash := loginThrottleHash(loginThrottleScopeClient, normalizeClientIP(oldIP))

	if _, err := testStore.ReserveLoginAttempt(t.Context(), oldUsername, oldIP, start); err != nil {
		t.Fatal(err)
	}
	if got := loginThrottleKeyCount(t, oldAccountHash, oldClientHash); got != 2 {
		t.Fatalf("initial expired-key rows = %d, want 2", got)
	}

	next := start.Add(loginThrottleRetention + time.Microsecond)
	if _, err := testStore.ReserveLoginAttempt(
		t.Context(),
		throttleTestUsername("expired-new"),
		throttleTestIP(),
		next,
	); err != nil {
		t.Fatal(err)
	}
	if got := loginThrottleKeyCount(t, oldAccountHash, oldClientHash); got != 0 {
		t.Fatalf("expired-key rows after cleanup = %d, want 0", got)
	}
}

type reservationResult struct {
	admission LoginAttemptAdmission
	err       error
}

func runConcurrentReservations(
	t *testing.T,
	total int,
	keys func(int) (string, string),
	now time.Time,
) (int, int) {
	t.Helper()

	start := make(chan struct{})
	results := make(chan reservationResult, total)
	var wg sync.WaitGroup
	wg.Add(total)

	for index := range total {
		username, clientIP := keys(index)
		go func() {
			defer wg.Done()
			<-start
			admission, err := testStore.ReserveLoginAttempt(t.Context(), username, clientIP, now)
			results <- reservationResult{admission: admission, err: err}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	var admitted, denied int
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.admission.RetryAfter > 0 {
			denied++
		} else {
			admitted++
		}
	}
	return admitted, denied
}

func throttleTestUsername(prefix string) string {
	return fmt.Sprintf("  %s-%s  ", prefix, uuid.NewString())
}

func throttleTestIP() string {
	id := uuid.New()
	return fmt.Sprintf(" 203.0.%d.%d ", id[0], id[1])
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

func loginThrottleAttemptCount(t *testing.T, scope, keyHash string) int32 {
	t.Helper()

	var count int32
	if err := testPool.QueryRow(
		t.Context(),
		"SELECT attempt_count FROM login_throttles WHERE scope = $1 AND key_hash = $2",
		scope,
		keyHash,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
