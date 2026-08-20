# OWASP Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close all eight approved OWASP audit findings without adding external infrastructure or weakening existing transfer and refresh-token guarantees.

**Architecture:** PostgreSQL remains the shared consistency boundary for login throttling and session revocation through the existing wide `internal/db.Store` seam. HTTP handlers expose dedicated response DTOs, Echo applies one explicit browser-security policy, and build/CI dependencies become immutable through checksums, image digests, and full action SHAs.

**Tech Stack:** Go 1.26, Echo v5, echo-jwt v5, PostgreSQL 18, pgx, sqlc, Goose, Svelte 5, TypeScript 6, Vitest, Playwright, Docker BuildKit, GitHub Actions, mise.

**Spec:** `docs/superpowers/specs/2026-08-20-owasp-hardening-design.md`

## Global Constraints

- Preserve `internal/db.Store` as the wide database seam; do not add handler-local store interfaces.
- Keep transaction orchestration in handwritten `internal/db` files; never edit `internal/db/sqlc/` manually.
- Require registration passwords from 15 through 72 bytes while MFA is absent.
- Return generic `202 Accepted` for successful registration, existing usernames, and existing emails.
- Keep access and refresh tokens memory/cookie scoped as currently designed; do not introduce browser-readable token storage.
- Preserve refresh-token hashing, rotation, stable session IDs, and transfer idempotency semantics.
- Keep API and SPA same-origin; do not add CORS.
- Use PostgreSQL, not Redis or proxy-specific state, for shared login throttling.
- Apply HSTS only when Echo identifies HTTPS directly or through `X-Forwarded-Proto: https`.
- Pin third-party GitHub Actions to full 40-character commit SHAs and retain version comments.
- Run `mise run sqlc:generate` after SQL changes and review generated diffs.
- Use focused tests during each task, then run every gate listed in Task 8.

---

## File Structure

### New files

- `internal/db/migrations/00005_login_throttles.sql` — login-throttle schema and rollback.
- `internal/db/query/login_throttles.sql` — sqlc persistence operations for throttle rows.
- `internal/db/login_throttle.go` — normalization, hashing, policy, cooldown calculation, and `Store` methods.
- `internal/db/login_throttle_test.go` — PostgreSQL integration coverage, including cross-store visibility.
- `internal/db/session.go` — server-session validation for access JWTs.
- `internal/api/security_headers_test.go` — exact browser-security header assertions.
- `frontend/src/lib/pages/RegisterPage.test.ts` — registration password metadata and copy coverage.
- `scripts/check-supply-chain.sh` — fail-closed checks for action SHAs, image digests, and verified mise installation.
- `docs/decisions/0007-use-postgresql-for-revocable-access-and-login-throttling.md` — durable authentication architecture decision.

### Modified source files

- `internal/db/store.go` — expose throttle and access-session operations through `Store`.
- `internal/db/sqlc/*.go` — regenerated only by sqlc.
- `internal/api/user.go` — generic registration, shared login throttle, and shared token/session ID.
- `internal/api/middleware.go` — validate access JWTs against PostgreSQL sessions.
- `internal/api/routes.go` — remove login from process-local limiter.
- `internal/api/transfer.go` — map internal transaction result to public DTO.
- `internal/api/server.go` — explicit Echo secure middleware configuration.
- `internal/api/user_test.go` — fake store seams, stronger test password, registration/login tests.
- `internal/api/auth_test.go` — session-backed access-token tests.
- `internal/api/token_renew_test.go` — shared token/session ID assertions.
- `internal/api/transfer_test.go` — response privacy assertions.
- `internal/api/spa_test.go` — SPA header assertions where useful.
- `frontend/src/lib/components/TextField.svelte` — string length input attributes.
- `frontend/src/lib/components/TextField.test.ts` — attribute propagation test.
- `frontend/src/lib/pages/RegisterPage.svelte` — 15-character hint and input constraints.
- `frontend/src/lib/api/types.ts` — narrow `TransferResult`.
- `README.md` — current registration/authentication behavior.
- `Dockerfile` — digest-pinned images and checksum-verified mise binary.
- `.github/workflows/ci.yml` — action SHAs and supply-chain check job/step.
- `.github/workflows/release.yml` — action SHAs.
- `.github/actions/go-cache/action.yml` — action SHAs.
- `mise.toml` — supply-chain verification task.
- `docs/decisions/README.md` — ADR index.

---

### Task 1: PostgreSQL-Backed Login Throttle

**Files:**
- Create: `internal/db/migrations/00005_login_throttles.sql`
- Create: `internal/db/query/login_throttles.sql`
- Create: `internal/db/login_throttle.go`
- Create: `internal/db/login_throttle_test.go`
- Modify: `internal/db/store.go:13-21`
- Generate: `internal/db/sqlc/login_throttles.sql.go`
- Generate: `internal/db/sqlc/models.go`
- Generate: `internal/db/sqlc/querier.go`

**Interfaces:**
- Produces:

```go
type LoginThrottleDecision struct {
	RetryAfter time.Duration
}

func (s *SQLStore) CheckLoginThrottle(
	ctx context.Context,
	username string,
	clientIP string,
	now time.Time,
) (LoginThrottleDecision, error)

func (s *SQLStore) RecordLoginFailure(
	ctx context.Context,
	username string,
	clientIP string,
	now time.Time,
) (LoginThrottleDecision, error)

func (s *SQLStore) ClearLoginAccountThrottle(
	ctx context.Context,
	username string,
) error
```

- Policy:
  - Account: 5 failures per 15 minutes.
  - Client IP: 20 failures per 15 minutes.
  - First cooldown: 30 seconds.
  - Each subsequent failure doubles cooldown.
  - Maximum cooldown: 15 minutes.
  - Rows expire 30 minutes after their latest update.

- [ ] **Step 1: Write migration**

```sql
-- +goose Up
CREATE TABLE login_throttles (
    scope             text NOT NULL CHECK (scope IN ('account', 'client')),
    key_hash          text NOT NULL,
    failure_count     integer NOT NULL CHECK (failure_count > 0),
    window_started_at timestamptz NOT NULL,
    blocked_until     timestamptz,
    expires_at        timestamptz NOT NULL,
    PRIMARY KEY (scope, key_hash)
);

CREATE INDEX idx_login_throttles_expires_at
    ON login_throttles (expires_at);

-- +goose Down
DROP TABLE login_throttles;
```

- [ ] **Step 2: Write sqlc queries**

```sql
-- name: GetLoginThrottle :one
SELECT *
FROM login_throttles
WHERE scope = $1 AND key_hash = $2
LIMIT 1;

-- name: IncrementLoginThrottle :one
INSERT INTO login_throttles (
    scope,
    key_hash,
    failure_count,
    window_started_at,
    blocked_until,
    expires_at
)
VALUES (
    sqlc.arg(scope),
    sqlc.arg(key_hash),
    1,
    sqlc.arg(now),
    NULL,
    sqlc.arg(now) + make_interval(secs => sqlc.arg(retention_seconds)::integer)
)
ON CONFLICT (scope, key_hash) DO UPDATE
SET failure_count = CASE
        WHEN login_throttles.window_started_at <=
             sqlc.arg(now) - make_interval(secs => sqlc.arg(window_seconds)::integer)
            THEN 1
        ELSE login_throttles.failure_count + 1
    END,
    window_started_at = CASE
        WHEN login_throttles.window_started_at <=
             sqlc.arg(now) - make_interval(secs => sqlc.arg(window_seconds)::integer)
            THEN sqlc.arg(now)
        ELSE login_throttles.window_started_at
    END,
    blocked_until = CASE
        WHEN login_throttles.window_started_at <=
             sqlc.arg(now) - make_interval(secs => sqlc.arg(window_seconds)::integer)
            THEN NULL
        ELSE login_throttles.blocked_until
    END,
    expires_at = sqlc.arg(now) +
        make_interval(secs => sqlc.arg(retention_seconds)::integer)
RETURNING *;

-- name: SetLoginThrottleBlockedUntil :one
UPDATE login_throttles
SET blocked_until = GREATEST(
        COALESCE(blocked_until, sqlc.arg(blocked_until)),
        sqlc.arg(blocked_until)
    ),
    expires_at = GREATEST(expires_at, sqlc.arg(blocked_until))
WHERE scope = sqlc.arg(scope)
  AND key_hash = sqlc.arg(key_hash)
RETURNING *;

-- name: DeleteLoginThrottle :exec
DELETE FROM login_throttles
WHERE scope = $1 AND key_hash = $2;

-- name: DeleteExpiredLoginThrottles :execrows
DELETE FROM login_throttles
WHERE expires_at <= $1;
```

- [ ] **Step 3: Generate sqlc code**

Run: `mise run sqlc:generate`

Expected: new generated query methods and `LoginThrottle` model; no manual edits under `internal/db/sqlc/`.

- [ ] **Step 4: Write failing integration tests**

Cover these exact cases in `internal/db/login_throttle_test.go`:

```go
func TestLoginThrottle_AccountBlocksAtFifthFailure(t *testing.T)
func TestLoginThrottle_ClientBlocksAtTwentiethFailure(t *testing.T)
func TestLoginThrottle_CooldownDoublesAndCaps(t *testing.T)
func TestLoginThrottle_WindowResetStartsAtOne(t *testing.T)
func TestLoginThrottle_ClearAccountPreservesClient(t *testing.T)
func TestLoginThrottle_IsSharedAcrossStoreInstances(t *testing.T)
func TestLoginThrottle_ExpiredRowsAreDeleted(t *testing.T)
```

Use unique usernames and IPs per test. For cross-instance coverage:

```go
otherStore := New(testPool)
now := time.Now().UTC()

for range 5 {
	if _, err := testStore.RecordLoginFailure(
		t.Context(), username, "203.0.113.10", now,
	); err != nil {
		t.Fatal(err)
	}
}

decision, err := otherStore.CheckLoginThrottle(
	t.Context(), username, "203.0.113.10", now,
)
if err != nil {
	t.Fatal(err)
}
if decision.RetryAfter <= 0 {
	t.Fatal("second store instance must observe account cooldown")
}
```

- [ ] **Step 5: Run focused integration tests to verify failure**

Run:

```bash
mise run compose:test:up
go test -tags=integration ./internal/db -run LoginThrottle -v
mise run compose:test:down
```

Expected: FAIL because `Store` methods do not exist.

- [ ] **Step 6: Implement key normalization and hashing**

Use lowercase trimmed usernames and canonical parsed IPs. Persist only SHA-256
hex digests:

```go
func loginThrottleHash(scope, raw string) string {
	sum := sha256.Sum256([]byte(scope + "\x00" + raw))
	return hex.EncodeToString(sum[:])
}

func normalizeLoginUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func normalizeClientIP(clientIP string) string {
	if ip := net.ParseIP(strings.TrimSpace(clientIP)); ip != nil {
		return ip.String()
	}
	return strings.TrimSpace(clientIP)
}
```

- [ ] **Step 7: Implement policy and cooldown calculation**

```go
const (
	loginThrottleWindow       = 15 * time.Minute
	loginThrottleBaseCooldown = 30 * time.Second
	loginThrottleMaxCooldown  = 15 * time.Minute
	loginThrottleRetention    = 30 * time.Minute
	accountFailureThreshold   = int32(5)
	clientFailureThreshold    = int32(20)
)

func loginCooldown(failures, threshold int32) time.Duration {
	if failures < threshold {
		return 0
	}
	exponent := min(failures-threshold, 10)
	cooldown := loginThrottleBaseCooldown * time.Duration(1<<exponent)
	return min(cooldown, loginThrottleMaxCooldown)
}
```

- [ ] **Step 8: Implement store methods**

`RecordLoginFailure` must run `DeleteExpiredLoginThrottles`, increment account
then client rows, calculate cooldowns, and persist `blocked_until` inside one
`execTx`. `CheckLoginThrottle` reads both keys and returns the greater positive
`blocked_until - now`. Missing rows are allowed; other database errors propagate.
`ClearLoginAccountThrottle` deletes only the account key.

- [ ] **Step 9: Add methods to `Store`**

```go
CheckLoginThrottle(
	context.Context, string, string, time.Time,
) (LoginThrottleDecision, error)
RecordLoginFailure(
	context.Context, string, string, time.Time,
) (LoginThrottleDecision, error)
ClearLoginAccountThrottle(context.Context, string) error
```

- [ ] **Step 10: Run focused integration tests**

Run:

```bash
mise run compose:test:up
go test -race -tags=integration ./internal/db -run LoginThrottle -v
mise run compose:test:down
```

Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add internal/db/migrations/00005_login_throttles.sql \
  internal/db/query/login_throttles.sql \
  internal/db/login_throttle.go \
  internal/db/login_throttle_test.go \
  internal/db/store.go \
  internal/db/sqlc
git commit -m "feat: add shared login throttling"
```

---

### Task 2: Enforce Shared Throttling in Login Flow

**Files:**
- Modify: `internal/api/user.go:136-197`
- Modify: `internal/api/routes.go:13-20`
- Modify: `internal/api/user_test.go:18-123,274-490`
- Modify: `internal/api/errors_test.go`

**Interfaces:**
- Consumes: `Store.CheckLoginThrottle`, `Store.RecordLoginFailure`, and `Store.ClearLoginAccountThrottle` from Task 1.
- Produces: generic `429` response with integer `Retry-After` seconds.

- [ ] **Step 1: Extend `fakeStore`**

Add optional function fields and methods:

```go
checkLoginThrottle func(
	context.Context, string, string, time.Time,
) (store.LoginThrottleDecision, error)
recordLoginFailure func(
	context.Context, string, string, time.Time,
) (store.LoginThrottleDecision, error)
clearLoginAccountThrottle func(context.Context, string) error
```

When a callback is nil, each fake method returns the zero decision and nil.
This keeps unrelated API tests focused on their own behavior.

- [ ] **Step 2: Write failing API tests**

Add:

```go
func TestLoginUserRejectsActiveAccountThrottle(t *testing.T)
func TestLoginUserRejectsActiveClientThrottle(t *testing.T)
func TestLoginUserRecordsUnknownUserFailure(t *testing.T)
func TestLoginUserRecordsWrongPasswordFailure(t *testing.T)
func TestLoginUserReturns429WhenFailureStartsCooldown(t *testing.T)
func TestLoginUserClearsAccountThrottleAfterValidCredentials(t *testing.T)
func TestLoginUserDoesNotClearClientThrottle(t *testing.T)
func TestLoginUserThrottleStoreErrorReturns500(t *testing.T)
```

For active throttles, assert:

```go
if rec.Code != http.StatusTooManyRequests {
	t.Fatalf("want 429, got %d (%s)", rec.Code, rec.Body.String())
}
if got := rec.Header().Get("Retry-After"); got != "30" {
	t.Fatalf("Retry-After = %q, want 30", got)
}
if !strings.Contains(rec.Body.String(), "too many login attempts") {
	t.Fatalf("unexpected body: %s", rec.Body.String())
}
```

- [ ] **Step 3: Run focused tests to verify failure**

Run: `go test ./internal/api -run 'LoginUser.*Throttle|LoginUserRecords|LoginUserClears' -v`

Expected: FAIL because login does not call shared throttle methods.

- [ ] **Step 4: Remove login from process-local limiter**

Keep `authLimiter` on registration, logout, renew, verification resend, and
verification links. Register login without it:

```go
v1.POST("/users/login", s.loginUser)
```

- [ ] **Step 5: Add throttle response helper**

```go
func loginThrottleError(c *echo.Context, retryAfter time.Duration) error {
	seconds := max(int64(math.Ceil(retryAfter.Seconds())), 1)
	c.Response().Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	return echo.NewHTTPError(http.StatusTooManyRequests, "too many login attempts")
}
```

- [ ] **Step 6: Add invalid-credential helper**

```go
func (s *Server) invalidLogin(
	c *echo.Context,
	username string,
) error {
	decision, err := s.store.RecordLoginFailure(
		c.Request().Context(),
		username,
		c.RealIP(),
		time.Now(),
	)
	if err != nil {
		return err
	}
	if decision.RetryAfter > 0 {
		return loginThrottleError(c, decision.RetryAfter)
	}
	return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
}
```

- [ ] **Step 7: Wire throttle checks into `loginUser`**

Order:

1. Validate request.
2. Call `CheckLoginThrottle`.
3. Reject active cooldown before user lookup.
4. Unknown user: perform dummy bcrypt comparison, then call `invalidLogin`.
5. Wrong password: call `invalidLogin`.
6. Valid credentials, including unverified users: clear account throttle.
7. Preserve `403 email verification required` for unverified users.
8. Continue token/session creation for verified users.

- [ ] **Step 8: Run focused API tests**

Run: `go test -race ./internal/api -run 'LoginUser' -v`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/api/user.go internal/api/routes.go \
  internal/api/user_test.go internal/api/errors_test.go
git commit -m "feat: enforce shared login throttling"
```

---

### Task 3: Bind Access JWTs to Revocable Sessions

**Files:**
- Create: `internal/db/session.go`
- Create: `docs/decisions/0007-use-postgresql-for-revocable-access-and-login-throttling.md`
- Modify: `docs/decisions/README.md`
- Modify: `internal/db/store.go`
- Modify: `internal/api/user.go:199-234`
- Modify: `internal/api/middleware.go:18-26`
- Modify: `internal/api/user_test.go`
- Modify: `internal/api/auth_test.go`
- Modify: `internal/api/token_renew_test.go`

**Interfaces:**
- Produces:

```go
func (s *SQLStore) ValidateAccessSession(
	ctx context.Context,
	id uuid.UUID,
	username string,
	now time.Time,
) error
```

- Access and refresh JWT `Payload.ID` values must equal the persisted session ID.

- [ ] **Step 1: Write failing store/API tests**

Add store integration cases:

```go
func TestValidateAccessSession_Valid(t *testing.T)
func TestValidateAccessSession_Missing(t *testing.T)
func TestValidateAccessSession_Blocked(t *testing.T)
func TestValidateAccessSession_Expired(t *testing.T)
func TestValidateAccessSession_UsernameMismatch(t *testing.T)
```

Add API cases:

```go
func TestProtectedRouteRejectsMissingSession(t *testing.T)
func TestProtectedRouteRejectsBlockedSession(t *testing.T)
func TestProtectedRouteRejectsExpiredSession(t *testing.T)
func TestProtectedRouteRejectsSessionUsernameMismatch(t *testing.T)
func TestProtectedRoutePropagatesSessionStoreError(t *testing.T)
func TestLogoutImmediatelyRevokesAccessToken(t *testing.T)
func TestIssueTokenPairUsesOneSessionID(t *testing.T)
```

- [ ] **Step 2: Run focused tests to verify failure**

Run:

```bash
go test ./internal/api -run 'ProtectedRoute|LogoutImmediately|IssueTokenPair' -v
mise run compose:test:up
go test -tags=integration ./internal/db -run ValidateAccessSession -v
mise run compose:test:down
```

Expected: FAIL because access JWTs are not session-backed.

- [ ] **Step 3: Implement `ValidateAccessSession`**

```go
func (s *SQLStore) ValidateAccessSession(
	ctx context.Context,
	id uuid.UUID,
	username string,
	now time.Time,
) error {
	session, err := s.GetSession(ctx, id)
	if err != nil {
		if errors.Is(ClassifyError(err), ErrRecordNotFound) {
			return ErrInvalidSession
		}
		return ClassifyError(err)
	}
	if session.IsBlocked ||
		session.Username != username ||
		!session.ExpiresAt.After(now) {
		return ErrInvalidSession
	}
	return nil
}
```

Add the method to `Store`.

- [ ] **Step 4: Make token pairs share one ID**

Rename the helper to express the new contract:

```go
func (s *Server) issueTokenPairWithSessionID(
	sessionID uuid.UUID,
	username string,
	role string,
) (tokenPair, error)
```

If `sessionID == uuid.Nil`, create one UUIDv7. Mint both tokens with
`CreateTokenWithID(sessionID, ...)`. Renewal passes the existing refresh/session
ID. Assert `accessPayload.ID == refreshPayload.ID`.

- [ ] **Step 5: Validate sessions in JWT success handler**

```go
SuccessHandler: func(c *echo.Context) error {
	payload, err := authPayload(c)
	if err != nil {
		return err
	}
	err = s.store.ValidateAccessSession(
		c.Request().Context(),
		payload.ID,
		payload.Username,
		time.Now(),
	)
	if errors.Is(err, store.ErrInvalidSession) {
		return echo.ErrUnauthorized
	}
	return err
},
```

Keep `NewClaimsFunc` expecting `token.Access`.

- [ ] **Step 6: Extend `fakeStore`**

Add an optional `validateAccessSession` callback. Return nil when it is absent
so existing authorization tests do not need irrelevant session setup. New
session lifecycle tests must set it explicitly and assert ID, username, and time.

- [ ] **Step 7: Prove logout revocation end to end in API tests**

Issue a token pair, use the access token successfully, call logout with the
refresh cookie, switch fake session state to blocked, then repeat the protected
request and assert `401`.

- [ ] **Step 8: Write ADR-0007**

Use existing ADR headings. Record:

- PostgreSQL session reads on every protected request.
- Stable shared ID across access JWT, refresh JWT, and session row.
- PostgreSQL account/IP throttle state.
- Rejected alternatives: stateless short TTL only, Redis, and proxy-only limits.
- Consequences: indexed read per protected request and shared cross-replica control.

Add ADR-0007 to `docs/decisions/README.md`.

- [ ] **Step 9: Run focused tests**

Run:

```bash
go test -race ./internal/api -run 'ProtectedRoute|Logout|IssueTokenPair|RenewToken' -v
mise run compose:test:up
go test -race -tags=integration ./internal/db -run 'ValidateAccessSession|RotateSession' -v
mise run compose:test:down
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/db/session.go internal/db/store.go \
  internal/api/user.go internal/api/middleware.go \
  internal/api/user_test.go internal/api/auth_test.go \
  internal/api/token_renew_test.go \
  docs/decisions/0007-use-postgresql-for-revocable-access-and-login-throttling.md \
  docs/decisions/README.md
git commit -m "feat: revoke access tokens with sessions"
```

---

### Task 4: Harden Registration Password and Enumeration Behavior

**Files:**
- Modify: `internal/api/user.go:46-50,95-134`
- Modify: `internal/api/user_test.go:125-272`
- Modify: `frontend/src/lib/components/TextField.svelte`
- Modify: `frontend/src/lib/components/TextField.test.ts`
- Modify: `frontend/src/lib/pages/RegisterPage.svelte`
- Create: `frontend/src/lib/pages/RegisterPage.test.ts`
- Modify: `README.md:130-131`

**Interfaces:**
- Registration password validation: `required,min=15,maxbytes=72`.
- `TextField` gains `minlength?: number` and `maxlength?: number`.
- All username/email collisions return `verificationAccepted` with status 202.

- [ ] **Step 1: Write failing backend tests**

Add:

```go
func TestCreateUserPasswordFourteenBytesRejected(t *testing.T)
func TestCreateUserPasswordFifteenBytesAccepted(t *testing.T)
func TestCreateUserUsernameExistsReturnsGenericAccepted(t *testing.T)
func TestCreateUserUsernameAndEmailResponsesMatch(t *testing.T)
```

Use:

```go
const testPassword = "correct horse battery staple"
```

Replace registration and login fixtures using `secret123` with `testPassword`
or JSON built from that constant.

- [ ] **Step 2: Run backend tests to verify failure**

Run: `go test ./internal/api -run 'CreateUser' -v`

Expected: 14-byte password accepted and duplicate username returns 409.

- [ ] **Step 3: Implement backend policy**

Change the tag:

```go
Password string `json:"password" validate:"required,min=15,maxbytes=72"`
```

Handle both collision sentinels generically:

```go
if errors.Is(classified, store.ErrUsernameExists) {
	return c.JSON(http.StatusAccepted, verificationAccepted)
}
if errors.Is(classified, store.ErrEmailExists) {
	// Preserve deduplicated registration notice.
	...
	return c.JSON(http.StatusAccepted, verificationAccepted)
}
```

- [ ] **Step 4: Run backend tests**

Run: `go test -race ./internal/api -run 'CreateUser|LoginUser' -v`

Expected: PASS.

- [ ] **Step 5: Write failing frontend tests**

`TextField.test.ts`:

```ts
it("forwards string length constraints", () => {
  render(TextField, {
    props: {
      id: "password",
      label: "Password",
      type: "password",
      value: "",
      minlength: 15,
      maxlength: 72,
    },
  });

  const field = screen.getByLabelText("Password");
  expect(field).toHaveAttribute("minlength", "15");
  expect(field).toHaveAttribute("maxlength", "72");
});
```

`RegisterPage.test.ts` must render the page and assert:

```ts
const field = screen.getByLabelText("Password");
expect(field).toHaveAttribute("minlength", "15");
expect(field).toHaveAttribute("maxlength", "72");
expect(screen.getByText("At least 15 characters.")).toBeInTheDocument();
```

- [ ] **Step 6: Run frontend tests to verify failure**

Run:

```bash
cd frontend
bun run test -- src/lib/components/TextField.test.ts src/lib/pages/RegisterPage.test.ts
```

Expected: FAIL because length attributes and copy are absent.

- [ ] **Step 7: Implement frontend constraints**

Add `minlength` and `maxlength` to `TextField` props, destructuring, and input
attributes. Pass `minlength={15}` and `maxlength={72}` from `RegisterPage`, and
change hint to `At least 15 characters.`

- [ ] **Step 8: Update README**

Change registration route description to state that it returns generic 202 for
accepted and colliding submissions and requires a 15–72 byte password.

- [ ] **Step 9: Run focused frontend checks**

Run:

```bash
mise run frontend:check
cd frontend
bun run test -- src/lib/components/TextField.test.ts src/lib/pages/RegisterPage.test.ts
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/api/user.go internal/api/user_test.go \
  frontend/src/lib/components/TextField.svelte \
  frontend/src/lib/components/TextField.test.ts \
  frontend/src/lib/pages/RegisterPage.svelte \
  frontend/src/lib/pages/RegisterPage.test.ts \
  README.md
git commit -m "fix: harden registration privacy"
```

---

### Task 5: Narrow Transfer API Response

**Files:**
- Modify: `internal/api/transfer.go:16-80`
- Modify: `internal/api/transfer_test.go:20-85`
- Modify: `frontend/src/lib/api/types.ts:63-68`
- Modify: `frontend/src/lib/pages/TransferPage.test.ts`

**Interfaces:**
- Produces:

```go
type transferResponse struct {
	Transfer    sqlcdb.Transfer `json:"transfer"`
	FromAccount sqlcdb.Account  `json:"from_account"`
}
```

```ts
export interface TransferResult {
  transfer: Transfer;
  from_account: Account;
}
```

- [ ] **Step 1: Write failing backend privacy test**

Return a complete internal `TransferTxResult` from the fake, including recipient
owner/balance and both entries. Decode the HTTP JSON as `map[string]json.RawMessage`:

```go
if _, ok := body["to_account"]; ok {
	t.Fatal("response exposed recipient account")
}
if _, ok := body["from_entry"]; ok {
	t.Fatal("response exposed source ledger entry")
}
if _, ok := body["to_entry"]; ok {
	t.Fatal("response exposed destination ledger entry")
}
if _, ok := body["transfer"]; !ok {
	t.Fatal("response missing transfer")
}
if _, ok := body["from_account"]; !ok {
	t.Fatal("response missing caller-owned source account")
}
```

- [ ] **Step 2: Run focused backend test to verify failure**

Run: `go test ./internal/api -run TestCreateTransferOK -v`

Expected: FAIL because raw `TransferTxResult` exposes all fields.

- [ ] **Step 3: Implement response DTO**

Map the result:

```go
return c.JSON(http.StatusOK, transferResponse{
	Transfer:    result.Transfer,
	FromAccount: result.FromAccount,
})
```

Do not change `store.TransferTxResult`.

- [ ] **Step 4: Narrow frontend type**

Remove `to_account` from `TransferResult`. Keep existing UI logic using only
`transfer` and `from_account`. Ensure mocked successful responses contain no
recipient account object.

- [ ] **Step 5: Run focused backend/frontend tests**

Run:

```bash
go test -race ./internal/api -run 'CreateTransfer' -v
cd frontend
bun run test -- src/lib/pages/TransferPage.test.ts
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/transfer.go internal/api/transfer_test.go \
  frontend/src/lib/api/types.ts frontend/src/lib/pages/TransferPage.test.ts
git commit -m "fix: hide recipient transfer data"
```

---

### Task 6: Apply Explicit Browser Security Headers

**Files:**
- Modify: `internal/api/server.go:57-62`
- Create: `internal/api/security_headers_test.go`
- Modify: `internal/api/spa_test.go`

**Interfaces:**
- CSP:

```text
default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; connect-src 'self'
```

- HSTS: `max-age=31536000; includeSubdomains`
- Referrer policy: `no-referrer`
- Frame policy: `DENY`
- Content type policy: `nosniff`

- [ ] **Step 1: Write failing header tests**

Add:

```go
func TestSecurityHeadersOnAPIResponse(t *testing.T)
func TestSecurityHeadersOnSPAResponse(t *testing.T)
func TestHSTSOnlyOnHTTPS(t *testing.T)
func TestHSTSBehindHTTPSProxy(t *testing.T)
```

For HTTPS:

```go
req := httptest.NewRequest(http.MethodGet, "https://simplebank.test/livez", nil)
```

For proxy HTTPS:

```go
req := httptest.NewRequest(http.MethodGet, "/livez", nil)
req.Header.Set(echo.HeaderXForwardedProto, "https")
```

Assert exact values for CSP, HSTS, Referrer-Policy, X-Frame-Options, and
X-Content-Type-Options. Plain HTTP must have no HSTS header.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/api -run 'SecurityHeaders|HSTS' -v`

Expected: FAIL because CSP, HSTS, and Referrer-Policy are absent.

- [ ] **Step 3: Configure Echo secure middleware**

```go
const contentSecurityPolicy = "default-src 'self'; base-uri 'self'; " +
	"object-src 'none'; frame-ancestors 'none'; form-action 'self'; " +
	"script-src 'self'; style-src 'self'; font-src 'self'; " +
	"img-src 'self' data:; connect-src 'self'"

e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
	XSSProtection:         "0",
	ContentTypeNosniff:    "nosniff",
	XFrameOptions:         "DENY",
	HSTSMaxAge:            31536000,
	ContentSecurityPolicy: contentSecurityPolicy,
	ReferrerPolicy:        "no-referrer",
}))
```

Do not duplicate these headers in Caddy.

- [ ] **Step 4: Run focused tests**

Run: `go test -race ./internal/api -run 'SecurityHeaders|HSTS|RegisterSPA' -v`

Expected: PASS.

- [ ] **Step 5: Build SPA and inspect generated HTML**

Run:

```bash
mise run frontend:build
rg '<script[^>]+src=|<link[^>]+href=|style=' frontend/dist/index.html
```

Expected: scripts, styles, and fonts resolve from same-origin assets; no inline
style attributes requiring CSP relaxation.

- [ ] **Step 6: Commit**

```bash
git add internal/api/server.go internal/api/security_headers_test.go \
  internal/api/spa_test.go
git commit -m "fix: enforce browser security headers"
```

---

### Task 7: Pin Container and CI Supply Chain

**Files:**
- Modify: `Dockerfile`
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `.github/actions/go-cache/action.yml`
- Create: `scripts/check-supply-chain.sh`
- Modify: `mise.toml`

**Interfaces:**
- mise release: `v2026.8.9`
- mise Linux x64 SHA-256: `997b6f3e0d760d292eb99f2824c70dddb432514b3a0d487f975c0f22b3cae430`
- mise Linux arm64 SHA-256: `8ad1ecc90aa40b234e96d77b62af94b6f40a59b4b1527dc1b075c007e54407c7`
- Debian digest: `sha256:3a39a0592364683e6bab97937b72cad5a8fa6dcbbee90edb3bb48c7f8e94f258`
- Distroless digest: `sha256:97b9d04bed1c754b756c3c4b6a04915c22fb0b5d96a59944eb3bf78c26e6e157`

- [ ] **Step 1: Write failing supply-chain checker**

Create executable `scripts/check-supply-chain.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

workflow_files=(
  .github/workflows/ci.yml
  .github/workflows/release.yml
  .github/actions/go-cache/action.yml
)

bad_actions="$(
  grep -HnE '^[[:space:]]*(- )?uses:' "${workflow_files[@]}" \
    | grep -Ev 'uses:[[:space:]]+\./|uses:[[:space:]]+[^@[:space:]]+@[0-9a-f]{40}([[:space:]]+#.*)?$' \
    || true
)"
if [[ -n "$bad_actions" ]]; then
  printf 'mutable GitHub Action references:\n%s\n' "$bad_actions" >&2
  exit 1
fi

if grep -Fq 'curl https://mise.run | sh' Dockerfile; then
  echo 'Dockerfile executes mutable mise installer' >&2
  exit 1
fi

grep -Fq 'sha256sum -c -' Dockerfile

from_count="$(grep -c '^FROM ' Dockerfile)"
pinned_from_count="$(grep -cE '^FROM .*@sha256:[0-9a-f]{64}' Dockerfile)"
if [[ "$from_count" != "$pinned_from_count" ]]; then
  echo 'every Dockerfile base image must be digest pinned' >&2
  exit 1
fi
```

- [ ] **Step 2: Add mise task and verify checker fails**

```toml
[tasks."supply-chain:check"]
description = "Verify immutable CI and container dependencies"
run = "scripts/check-supply-chain.sh"
```

Run: `chmod +x scripts/check-supply-chain.sh && mise run supply-chain:check`

Expected: FAIL on mutable action tags and installer.

- [ ] **Step 3: Replace remote installer**

Use:

```dockerfile
FROM --platform=$BUILDPLATFORM debian:trixie-slim@sha256:3a39a0592364683e6bab97937b72cad5a8fa6dcbbee90edb3bb48c7f8e94f258 AS builder

ARG BUILDARCH
ARG MISE_VERSION=v2026.8.9
ARG MISE_SHA256_X64=997b6f3e0d760d292eb99f2824c70dddb432514b3a0d487f975c0f22b3cae430
ARG MISE_SHA256_ARM64=8ad1ecc90aa40b234e96d77b62af94b6f40a59b4b1527dc1b075c007e54407c7

RUN case "$BUILDARCH" in \
      amd64) mise_arch=x64; mise_sha="$MISE_SHA256_X64" ;; \
      arm64) mise_arch=arm64; mise_sha="$MISE_SHA256_ARM64" ;; \
      *) echo "unsupported BUILDARCH: $BUILDARCH" >&2; exit 1 ;; \
    esac \
    && curl -fsSLo /usr/local/bin/mise \
      "https://github.com/jdx/mise/releases/download/${MISE_VERSION}/mise-${MISE_VERSION}-linux-${mise_arch}" \
    && echo "${mise_sha}  /usr/local/bin/mise" | sha256sum -c - \
    && chmod 0755 /usr/local/bin/mise
```

Pin runtime image:

```dockerfile
FROM gcr.io/distroless/base-debian13:nonroot@sha256:97b9d04bed1c754b756c3c4b6a04915c22fb0b5d96a59944eb3bf78c26e6e157
```

- [ ] **Step 4: Replace all action tags with exact SHAs**

Use this mapping everywhere, retaining comments:

```text
actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
jdx/mise-action@3c2e0cf82a5b2e5249f0d3635a4d83d0ae861518 # v4
actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 # v6
actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7
docker/setup-docker-action@77e84dbf09b47d1e29270283c22f16145aa85ca1 # v5
docker/setup-buildx-action@37fe631027851001ddb9b187196cc803df7f5f0e # v4
docker/setup-compose-action@4eb059ff7f16592f9c84d5ca339c53cb7c5064e2 # v2
docker/build-push-action@53b7df96c91f9c12dcc8a07bcb9ccacbed38856a # v7
docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4
docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411302 # v6
actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8
```

Local actions such as `uses: ./.github/actions/go-cache` remain unchanged.

- [ ] **Step 5: Run supply-chain checker**

Run: `mise run supply-chain:check`

Expected: PASS.

- [ ] **Step 6: Add checker to CI**

Add a `supply-chain` job after checkout, or add the command to the existing
lint job before tools execute. Prefer a dedicated job with `contents: read`:

```yaml
supply-chain:
  runs-on: ubuntu-latest
  timeout-minutes: 5
  steps:
    - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
    - run: scripts/check-supply-chain.sh
```

Make the Docker job depend on `supply-chain`.

- [ ] **Step 7: Build both image platforms**

Run:

```bash
source env.bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION="$VERSION" \
  --build-arg COMMIT="$COMMIT" \
  --build-arg BUILD_DATE="$BUILD_DATE" \
  .
```

Expected: both architectures download the matching mise binary and pass SHA-256 verification.

- [ ] **Step 8: Commit**

```bash
git add Dockerfile .github/workflows/ci.yml \
  .github/workflows/release.yml \
  .github/actions/go-cache/action.yml \
  scripts/check-supply-chain.sh mise.toml
git commit -m "build: pin supply chain inputs"
```

---

### Task 8: Full Verification and Final Review

**Files:**
- Review: all changed files
- No new implementation surface unless a gate reveals a regression caused by Tasks 1–7.

**Interfaces:**
- Consumes every task deliverable.
- Produces a fully verified branch and synchronized Codegraph index.

- [ ] **Step 1: Regenerate sqlc and reject drift**

Run:

```bash
mise run sqlc:generate
git diff --exit-code -- internal/db/sqlc
```

Expected: no uncommitted generated drift.

- [ ] **Step 2: Run backend formatting and lint**

Run:

```bash
mise run golangci-lint:fmt
mise run golangci-lint
```

Expected: PASS and no unexpected formatting diff.

- [ ] **Step 3: Run backend unit tests**

Run: `mise run test:unit`

Expected: PASS with race detector.

- [ ] **Step 4: Run PostgreSQL integration tests**

Run: `mise run test:integration`

Expected: PASS; Compose lifecycle cleans up through `depends_post`.

- [ ] **Step 5: Run frontend static gates**

Run:

```bash
mise run frontend:check
mise run frontend:lint
mise run frontend:format:check
```

Expected: PASS.

- [ ] **Step 6: Run frontend test gates**

Run:

```bash
mise run frontend:test
mise run frontend:test:e2e
```

Expected: PASS.

- [ ] **Step 7: Run build and security gates**

Run:

```bash
mise run supply-chain:check
mise run app:build
mise run govulncheck
```

Expected: PASS.

- [ ] **Step 8: Exercise runtime headers and revocation**

Start the HTTPS profile:

```bash
mise run compose:dev-https:up
```

Verify:

```bash
curl --cacert tls/rootCA.pem -I https://localhost:8443/
```

Expected headers include CSP, HSTS, `Referrer-Policy: no-referrer`,
`X-Frame-Options: DENY`, and `X-Content-Type-Options: nosniff`.

Log in through the API, retain the access token and refresh cookie, call logout,
then call `/api/v1/accounts` with the retained access token. Expected: `401`.

Stop profile:

```bash
mise run compose:dev-https:down
```

- [ ] **Step 9: Synchronize code intelligence**

Run:

```bash
codegraph sync /home/canhdinh/workspace/simplebank
codegraph status /home/canhdinh/workspace/simplebank
```

Expected: index current and healthy.

- [ ] **Step 10: Review final diff**

Run:

```bash
git --no-pager diff --check
git --no-pager status --short
git --no-pager diff HEAD~7 --stat
```

Confirm:

- no raw recipient account appears in transfer JSON;
- access and refresh token IDs equal the session ID;
- throttle keys are hashed before persistence;
- username/email collisions are indistinguishable;
- no mutable third-party action refs remain;
- no unverified remote installer remains;
- no required gate was skipped.

- [ ] **Step 11: Commit verification-only fixes if required**

If a gate exposes a regression, return to the task that introduced it, make the
focused correction, rerun that task's focused tests, stage the exact changed
paths reported by `git status --short`, and commit with
`fix: resolve hardening regressions`. If no files changed, do not create an
empty commit.
