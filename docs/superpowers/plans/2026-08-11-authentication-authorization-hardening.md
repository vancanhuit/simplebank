# Authentication and Authorization Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden SimpleBank's token, session, registration, transfer-replay, and demo-funding flows without removing intentional demo opening balances.

**Architecture:** Keep short-lived access JWTs in memory and move single-use refresh JWTs into host-only HttpOnly cookies backed by atomically rotated PostgreSQL sessions with stable session IDs. Enforce authorization at both HTTP and transaction boundaries: typed JWT purposes at middleware, source-scoped idempotency in SQL, verified-email login, privacy-preserving registration responses, and server-owned per-currency opening caps published to the SPA.

**Tech Stack:** Go 1.26, Echo v5, golang-jwt v5, pgx v5, sqlc, PostgreSQL 18, River, Svelte 5, TypeScript 6, Vitest, Bun, mise.

## Global Constraints

- Use only checked-in `mise` tasks for generation, tests, builds, lint, and vulnerability scans.
- Follow TDD: prove each reported vulnerability or boundary failure before changing production code.
- Access JWT TTL remains `15m`; refresh JWT TTL remains `24h` unless operator configuration overrides it.
- JWT verification continues to allow only HS256 and secrets of at least 32 characters.
- Refresh cookie is host-only, `HttpOnly`, `SameSite=Strict`, path `/api/v1`, and `Secure=true` by default.
- `SESSION_COOKIE_SECURE=false` is allowed only when `PUBLIC_BASE_URL` is not HTTPS.
- Refresh tokens never appear in JSON, browser storage, logs, or database plaintext.
- Registration and resend return the same `202` response for new and existing email addresses; username collisions remain `409`.
- Missing `ACCOUNT_OPENING_LIMITS` currency entries have cap `0`.
- Checked-in demo opening caps are USD `100000`, EUR `100000`, and VND `25000000` minor units.
- Preserve the existing `{"error":"..."}` error response shape.
- Do not hand-edit `internal/db/sqlc/*`; regenerate it with `mise run sqlc:generate`.
- Do not commit unless the user explicitly authorizes commits. Commit commands below are execution checkpoints, not implicit authorization.

## File Structure

- `internal/token/maker.go`: token-purpose types, payload validation, and `Maker` contract.
- `internal/token/jwt_maker.go`: typed JWT creation and expected-purpose verification.
- `internal/api/middleware.go`: access-only bearer middleware.
- `internal/db/session_tx.go`: one-time refresh-session rotation transaction.
- `internal/db/transfer_tx.go`: source-scoped replay and immutable-request comparison.
- `internal/api/session_cookie.go`: refresh-cookie read, set, and clear policy.
- `internal/api/user.go`: registration, login, renewal, logout, and resend orchestration.
- `internal/worker/verify_email.go`: bounded verification-email uniqueness.
- `internal/worker/registration_notice.go`: deduplicated existing-account security notice.
- `internal/config/config.go`: cookie policy and opening-limit configuration.
- `internal/api/meta.go`: public policy endpoints.
- `frontend/src/lib/stores/auth.svelte.ts`: cookie-backed session restoration and logout.
- `frontend/src/lib/opening-limits.ts`: pure opening-limit display and validation helpers.
- `frontend/src/lib/pages/NewAccountPage.svelte`: opening-limit loading and form enforcement.

---

### Task 1: Enforce JWT Purpose

**Files:**
- Modify: `internal/token/maker.go`
- Modify: `internal/token/jwt_maker.go`
- Modify: `internal/token/jwt_maker_test.go`
- Modify: `internal/api/middleware.go`
- Modify: `internal/api/auth_test.go`
- Modify: `internal/api/user.go`
- Modify: all Go test call sites returned by compilation after the `Maker` signature changes

**Interfaces:**
- Produces: `type Type string`, `const Access Type = "access"`, `const Refresh Type = "refresh"`.
- Produces: `CreateToken(username, role string, tokenType token.Type, duration time.Duration)`.
- Produces: `VerifyToken(raw string, expectedType token.Type)`.
- Produces: `NewExpectedPayload(expectedType Type) *Payload` for Echo middleware claims.

- [ ] **Step 1: Write failing token-purpose tests**

Add cases to `internal/token/jwt_maker_test.go` that require the expected purpose and reject cross-use:

```go
func TestJWTMakerRejectsWrongTokenType(t *testing.T) {
	t.Parallel()
	maker, err := NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}

	raw, _, err := maker.CreateToken("alice", "depositor", Refresh, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maker.VerifyToken(raw, Access); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("refresh token verified as access token: %v", err)
	}
}

func TestJWTMakerAcceptsExpectedTokenType(t *testing.T) {
	t.Parallel()
	maker, err := NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}

	raw, _, err := maker.CreateToken("alice", "depositor", Access, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := maker.VerifyToken(raw, Access)
	if err != nil {
		t.Fatal(err)
	}
	if payload.TokenType != Access {
		t.Fatalf("token type = %q, want %q", payload.TokenType, Access)
	}
}
```

Add the reported exploit regression to `internal/api/auth_test.go`: mint `token.Refresh`, send it as bearer to `GET /api/v1/accounts`, and require `401` before `ListAccounts` can run.

```go
func TestProtectedRouteRejectsRefreshToken(t *testing.T) {
	t.Parallel()
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := maker.CreateToken("alice", roleDepositor, token.Refresh, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	fake := fakeStore{
		listAccounts: func(context.Context, sqlcdb.ListAccountsParams) ([]sqlcdb.Account, error) {
			t.Fatal("refresh token must be rejected before handler execution")
			return nil, nil
		},
	}
	s := newTestServerWithStore(t, fake)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for refresh bearer, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run RED suite**

Run: `mise run test:unit`

Expected: FAIL because token purpose constants/signatures do not exist, or because refresh bearer currently reaches the protected handler.

- [ ] **Step 3: Implement typed claims and validation**

Change `internal/token/maker.go` to this contract:

```go
type Type string

const (
	Access  Type = "access"
	Refresh Type = "refresh"
)

type Payload struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	TokenType Type      `json:"token_type"`
	jwt.RegisteredClaims
	expectedType Type
}

func NewExpectedPayload(expectedType Type) *Payload {
	return &Payload{expectedType: expectedType}
}

func (p Payload) Validate() error {
	if p.expectedType != "" && p.TokenType != p.expectedType {
		return ErrInvalidToken
	}
	if p.TokenType != Access && p.TokenType != Refresh {
		return ErrInvalidToken
	}
	return nil
}

type Maker interface {
	CreateToken(username, role string, tokenType Type, duration time.Duration) (string, *Payload, error)
	VerifyToken(raw string, expectedType Type) (*Payload, error)
}
```

Update `NewPayload` to accept and set `tokenType`. Update `JWTMaker.VerifyToken` to parse `NewExpectedPayload(expectedType)` and preserve the existing HS256 allowlist and expired-token mapping.

Configure Echo claims with `token.NewExpectedPayload(token.Access)`:

```go
NewClaimsFunc: func(c *echo.Context) jwt.Claims {
	return token.NewExpectedPayload(token.Access)
},
```

Pass `token.Access` and `token.Refresh` from `issueTokenPair`, and update test helpers to mint access tokens explicitly.

- [ ] **Step 4: Run GREEN suite**

Run: `mise run test:unit`

Expected: PASS, including `TestJWTMakerRejectsWrongTokenType` and `TestProtectedRouteRejectsRefreshToken`.

- [ ] **Step 5: Review checkpoint**

Inspect: `git diff -- internal/token internal/api/middleware.go internal/api/auth_test.go internal/api/user.go`

Verify no token parser accepts an unspecified expected purpose.

- [ ] **Step 6: Commit only with explicit authorization**

```bash
git add internal/token internal/api/middleware.go internal/api/auth_test.go internal/api/user.go internal/api/*_test.go
git commit -m "fix(auth): enforce JWT token purpose"
```

---

### Task 2: Scope Transfer Idempotency To Source Account

**Files:**
- Create: `internal/db/migrations/00003_scope_transfer_idempotency.sql`
- Modify: `internal/db/query/transfers.sql`
- Regenerate: `internal/db/sqlc/transfers.sql.go`
- Regenerate: `internal/db/sqlc/querier.go`
- Modify: `internal/db/errors.go`
- Modify: `internal/db/errors_test.go`
- Modify: `internal/db/transfer_tx.go`
- Modify: `internal/db/transfer_safety_test.go`
- Modify: `internal/api/errors.go`
- Modify: `internal/api/errors_test.go`

**Interfaces:**
- Produces: `ErrIdempotencyConflict` mapped to HTTP `409`.
- Produces: sqlc `GetTransferBySourceAndIdempotencyKey(ctx, params)`.
- Preserves: `TransferTx(ctx, TransferTxParams) (TransferTxResult, error)`.

- [ ] **Step 1: Write failing integration regressions**

Add two integration tests to `internal/db/transfer_safety_test.go`:

```go
func TestTransferTxSameKeyDifferentSourceDoesNotReplay(t *testing.T) {
	firstOwner := createTestUser(t)
	secondOwner := createTestUser(t)
	recipient := createTestUser(t)
	firstSource := createTestAccount(t, firstOwner.Username)
	secondSource := createTestAccount(t, secondOwner.Username)
	destination := createTestAccount(t, recipient.Username)
	key := uuid.New()

	first, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID: firstSource.ID, ToAccountID: destination.ID,
		Amount: 10, Currency: currency.USD, IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID: secondSource.ID, ToAccountID: destination.ID,
		Amount: 20, Currency: currency.USD, IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Transfer.ID == first.Transfer.ID || second.Transfer.FromAccountID != secondSource.ID {
		t.Fatalf("cross-source key replayed original transfer: %+v", second.Transfer)
	}
}

func TestTransferTxRejectsMismatchedReplay(t *testing.T) {
	owner := createTestUser(t)
	recipientA := createTestUser(t)
	recipientB := createTestUser(t)
	source := createTestAccount(t, owner.Username)
	destinationA := createTestAccount(t, recipientA.Username)
	destinationB := createTestAccount(t, recipientB.Username)
	key := uuid.New()

	_, err := testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID: source.ID, ToAccountID: destinationA.ID,
		Amount: 10, Currency: currency.USD, IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = testStore.TransferTx(t.Context(), TransferTxParams{
		FromAccountID: source.ID, ToAccountID: destinationB.ID,
		Amount: 10, Currency: currency.USD, IdempotencyKey: key,
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("want ErrIdempotencyConflict, got %v", err)
	}
}
```

- [ ] **Step 2: Run RED integration suite**

Run: `mise run test:integration`

Expected: FAIL because global uniqueness prevents same-key/different-source transfer and mismatched replay returns original data.

- [ ] **Step 3: Add composite uniqueness migration and scoped query**

Create `00003_scope_transfer_idempotency.sql`:

```sql
-- +goose Up
ALTER TABLE transfers DROP CONSTRAINT transfers_idempotency_key_key;
ALTER TABLE transfers
    ADD CONSTRAINT transfers_from_account_id_idempotency_key_key
    UNIQUE (from_account_id, idempotency_key);

-- +goose Down
ALTER TABLE transfers DROP CONSTRAINT transfers_from_account_id_idempotency_key_key;
ALTER TABLE transfers
    ADD CONSTRAINT transfers_idempotency_key_key UNIQUE (idempotency_key);
```

Replace `GetTransferByIdempotencyKey` with:

```sql
-- name: GetTransferBySourceAndIdempotencyKey :one
SELECT * FROM transfers
WHERE from_account_id = sqlc.arg(from_account_id)
  AND idempotency_key = sqlc.arg(idempotency_key)
LIMIT 1;
```

Run: `mise run sqlc:generate`

Expected: generated query accepts both source account ID and key.

- [ ] **Step 4: Implement replay comparison**

Add `ErrIdempotencyConflict = errors.New("idempotency key reused with different transfer parameters")` and map it to `409` with client message `"idempotency key conflicts with an existing transfer"`.

Use the scoped query in both the fast path and unique-conflict path. Before returning replay data, compare all request-bound immutable fields:

```go
func (s *SQLStore) replayTransfer(
	ctx context.Context,
	existing sqlcdb.Transfer,
	arg TransferTxParams,
) (TransferTxResult, error) {
	if existing.FromAccountID != arg.FromAccountID ||
		existing.ToAccountID != arg.ToAccountID ||
		existing.Amount != arg.Amount {
		return TransferTxResult{}, ErrIdempotencyConflict
	}

	fromAccount, err := s.GetAccount(ctx, existing.FromAccountID)
	if err != nil {
		return TransferTxResult{}, ClassifyError(err)
	}
	toAccount, err := s.GetAccount(ctx, existing.ToAccountID)
	if err != nil {
		return TransferTxResult{}, ClassifyError(err)
	}
	if fromAccount.Currency != arg.Currency || toAccount.Currency != arg.Currency {
		return TransferTxResult{}, ErrIdempotencyConflict
	}
	return TransferTxResult{
		Transfer: existing, FromAccount: fromAccount, ToAccount: toAccount,
	}, nil
}
```

- [ ] **Step 5: Run GREEN integration suite**

Run: `mise run test:integration`

Expected: PASS, including same-key/different-source and mismatched-replay cases.

- [ ] **Step 6: Run unit suite for HTTP mapping**

Run: `mise run test:unit`

Expected: PASS with `ErrIdempotencyConflict` mapped to `409`.

- [ ] **Step 7: Commit only with explicit authorization**

```bash
git add internal/db/migrations/00003_scope_transfer_idempotency.sql internal/db/query/transfers.sql internal/db/sqlc internal/db/errors.go internal/db/errors_test.go internal/db/transfer_tx.go internal/db/transfer_safety_test.go internal/api/errors.go internal/api/errors_test.go
git commit -m "fix(transfers): scope idempotency to source"
```

---

### Task 3: Add Atomic Session Rotation

**Files:**
- Modify: `internal/db/query/sessions.sql`
- Regenerate: `internal/db/sqlc/sessions.sql.go`
- Regenerate: `internal/db/sqlc/querier.go`
- Create: `internal/db/session_tx.go`
- Create: `internal/db/session_tx_test.go`
- Modify: `internal/db/store.go`
- Modify: `internal/db/errors.go`
- Modify: `internal/db/errors_test.go`

**Interfaces:**
- Produces: `ErrInvalidSession`.
- Produces: `SessionReplacement{ID uuid.UUID, RefreshTokenHash string, ExpiresAt time.Time}`.
- Produces: `RotateSessionTx(ctx, RotateSessionTxParams) (sqlcdb.Session, error)`.
- Produces: sqlc `BlockSession(ctx, BlockSessionParams)`.

- [ ] **Step 1: Write failing integration tests for one-time rotation**

Create `internal/db/session_tx_test.go` with the integration build tag. Cover valid rotation, blocked session, expired session, token-hash mismatch, and concurrent reuse. The concurrency assertion must require exactly one success:

```go
func TestRotateSessionTxConcurrentReuse(t *testing.T) {
	user := createTestUser(t)
	oldID := uuid.New()
	const oldHash = "old-refresh-hash"
	_, err := testStore.CreateSession(t.Context(), sqlcdb.CreateSessionParams{
		ID: oldID, Username: user.Username, RefreshToken: oldHash,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := testStore.RotateSessionTx(t.Context(), RotateSessionTxParams{
				ID: oldID, Username: user.Username, RefreshTokenHash: oldHash,
				Now: time.Now(),
				NewSession: func() (SessionReplacement, error) {
					return SessionReplacement{
						ID: oldID, RefreshTokenHash: uuid.NewString(),
						ExpiresAt: time.Now().Add(time.Hour),
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
```

- [ ] **Step 2: Run RED integration suite**

Run: `mise run test:integration`

Expected: FAIL because `RotateSessionTx` and session-lock/update queries do not exist.

- [ ] **Step 3: Add session queries and regenerate sqlc**

Append to `internal/db/query/sessions.sql`:

```sql
-- name: GetSessionForUpdate :one
SELECT * FROM sessions WHERE id = $1 LIMIT 1 FOR UPDATE;

-- name: RotateSession :one
UPDATE sessions
SET refresh_token = sqlc.arg(new_refresh_token),
    expires_at = sqlc.arg(new_expires_at)
WHERE id = sqlc.arg(old_id)
RETURNING *;

-- name: BlockSession :one
UPDATE sessions
SET is_blocked = true
WHERE id = sqlc.arg(id)
RETURNING *;
```

Run: `mise run sqlc:generate`

- [ ] **Step 4: Implement rotation transaction**

Create `internal/db/session_tx.go`:

```go
type SessionReplacement struct {
	ID               uuid.UUID
	RefreshTokenHash string
	ExpiresAt        time.Time
}

type RotateSessionTxParams struct {
	ID               uuid.UUID
	Username         string
	RefreshTokenHash string
	Now              time.Time
	NewSession       func() (SessionReplacement, error)
}

func (s *SQLStore) RotateSessionTx(ctx context.Context, arg RotateSessionTxParams) (sqlcdb.Session, error) {
	var rotated sqlcdb.Session
	err := s.execTx(ctx, func(q *sqlcdb.Queries) error {
		session, err := q.GetSessionForUpdate(ctx, arg.ID)
		if err != nil {
			if errors.Is(ClassifyError(err), ErrRecordNotFound) {
				return ErrInvalidSession
			}
			return ClassifyError(err)
		}
		tokenMatches := subtle.ConstantTimeCompare(
			[]byte(session.RefreshToken), []byte(arg.RefreshTokenHash),
		) == 1
		if session.IsBlocked || session.Username != arg.Username ||
			!tokenMatches || !session.ExpiresAt.After(arg.Now) {
			return ErrInvalidSession
		}
		replacement, err := arg.NewSession()
		if err != nil {
			return err
		}
		if replacement.ID != session.ID {
			return ErrSessionIDMismatch
		}
		rotated, err = q.RotateSession(ctx, sqlcdb.RotateSessionParams{
			OldID: session.ID,
			NewRefreshToken: replacement.RefreshTokenHash,
			NewExpiresAt: replacement.ExpiresAt,
		})
		return ClassifyError(err)
	})
	return rotated, err
}
```

Add the method to `Store`. Add `ErrInvalidSession` and unit classification coverage without exposing hash values in errors.

- [ ] **Step 5: Run GREEN integration suite**

Run: `mise run test:integration`

Expected: PASS; concurrent reuse has one success and one `ErrInvalidSession`.

- [ ] **Step 6: Commit only with explicit authorization**

```bash
git add internal/db/query/sessions.sql internal/db/sqlc internal/db/session_tx.go internal/db/session_tx_test.go internal/db/store.go internal/db/errors.go internal/db/errors_test.go
git commit -m "feat(auth): rotate refresh sessions atomically"
```

---

### Task 4: Move Refresh Tokens Into Cookies And Add Logout

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Create: `internal/api/session_cookie.go`
- Create: `internal/api/session_cookie_test.go`
- Modify: `internal/api/user.go`
- Modify: `internal/api/user_test.go`
- Modify: `internal/api/token_renew_test.go`
- Modify: `internal/api/routes.go`
- Modify: `internal/api/errors.go`
- Modify: `compose.yaml`

**Interfaces:**
- Produces config: `SessionCookieSecure bool` from `SESSION_COOKIE_SECURE` / `--session-cookie-secure`, default `true`.
- Produces cookie name: `simplebank_refresh`.
- Changes login JSON: removes `refresh_token` and `refresh_token_expires_at`.
- Changes renew request: no JSON body; reads refresh cookie.
- Changes renew JSON: `access_token`, `access_token_expires_at`, and `user`.
- Produces: `POST /api/v1/users/logout`, idempotent `204`.

- [ ] **Step 1: Write failing cookie and handler tests**

Add tests requiring:

1. Login sets `simplebank_refresh` with `HttpOnly`, `SameSite=Strict`, path `/api/v1`, expected `Secure` value, and no refresh token in JSON.
2. Renew rejects missing cookie with `401`.
3. Renew passes the cookie hash into `RotateSessionTx`, returns the current user, and replaces the cookie.
4. Reusing the old cookie returns `401` after the fake store reports `ErrInvalidSession`.
5. Logout calls `BlockSession` with JWT ID plus hash, expires the cookie, and returns `204`.
6. Logout without a cookie also returns `204`.

The login response assertion must decode into a map and prove the secret fields are absent:

```go
var body map[string]any
if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
	t.Fatal(err)
}
if _, ok := body["refresh_token"]; ok {
	t.Fatal("login JSON exposed refresh token")
}
cookie := rec.Result().Cookies()[0]
if cookie.Name != refreshCookieName || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
	t.Fatalf("unsafe refresh cookie: %+v", cookie)
}
```

- [ ] **Step 2: Run RED unit suite**

Run: `mise run test:unit`

Expected: FAIL because login returns refresh JSON, renew requires a body, and logout/cookie helpers do not exist.

- [ ] **Step 3: Add explicit cookie configuration**

Add `SessionCookieSecure bool` to `Config`, a boolean flag with `Value: true`, and this validation:

```go
if c.PublicBaseURL != "" {
	baseURL, err := url.Parse(c.PublicBaseURL)
	if err != nil {
		return fmt.Errorf("invalid public-base-url: %w", err)
	}
	if strings.EqualFold(baseURL.Scheme, "https") && !c.SessionCookieSecure {
		return errors.New("session-cookie-secure must be true for an HTTPS public-base-url")
	}
}
```

Add table cases for HTTPS plus insecure cookie rejection and HTTP plus insecure cookie acceptance. Set `SESSION_COOKIE_SECURE: "false"` only on `app-dev`; HTTPS and proxy profiles retain default `true`.

- [ ] **Step 4: Implement cookie policy helpers**

Create `internal/api/session_cookie.go`:

```go
const refreshCookieName = "simplebank_refresh"

func (s *Server) setRefreshCookie(c *echo.Context, raw string, expiresAt time.Time) {
	c.SetCookie(&http.Cookie{
		Name: refreshCookieName, Value: raw, Path: "/api/v1",
		Expires: expiresAt, HttpOnly: true, Secure: s.config.SessionCookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearRefreshCookie(c *echo.Context) {
	c.SetCookie(&http.Cookie{
		Name: refreshCookieName, Value: "", Path: "/api/v1",
		Expires: time.Unix(1, 0), MaxAge: -1, HttpOnly: true,
		Secure: s.config.SessionCookieSecure, SameSite: http.SameSiteStrictMode,
	})
}
```

Use `c.Cookie(refreshCookieName)` to read it. Map a missing cookie to `echo.ErrUnauthorized` only for renew; logout handles it as already complete.

- [ ] **Step 5: Implement login, renewal, and logout contracts**

Login must set the refresh cookie after session creation and return only access data, session ID, and profile.

Renew verifies `token.Refresh`, then calls `RotateSessionTx`. Mint both replacement tokens inside `NewSession` so token creation happens only after the old row is locked and validated:

```go
var tokens tokenPair
_, err = s.store.RotateSessionTx(ctx, store.RotateSessionTxParams{
	ID: refreshPayload.ID,
	Username: refreshPayload.Username,
	RefreshTokenHash: hashRefreshToken(cookie.Value),
	Now: time.Now(),
	NewSession: func() (store.SessionReplacement, error) {
		var err error
		tokens, err = s.issueTokenPairWithRefreshID(
			refreshPayload.ID, refreshPayload.Username, refreshPayload.Role,
		)
		if err != nil {
			return store.SessionReplacement{}, err
		}
		return store.SessionReplacement{
			ID: tokens.refreshPayload.ID,
			RefreshTokenHash: hashRefreshToken(tokens.refresh),
			ExpiresAt: tokens.refreshPayload.ExpiresAt.Time,
		}, nil
	},
})
```

After commit, load `GetUser(refreshPayload.Username)`, set replacement cookie, and return `renewTokenResponse{AccessToken, AccessTokenExpiresAt, User}`.

Logout always clears the cookie first. For a valid refresh JWT, call generated `BlockSession` with the stable session ID. The row lock serializes blocking against renewal, so logout revokes an in-flight replacement regardless of which operation wins first. Ignore `ErrRecordNotFound`; return classified infrastructure errors. Register `POST /users/logout` under public `v1` because authentication comes from the refresh cookie, not access middleware.

Map `store.ErrInvalidSession` to `401` with client message `"invalid session"` in the central API error catalog so renew never leaks which session check failed.

- [ ] **Step 6: Run GREEN unit suite**

Run: `mise run test:unit`

Expected: PASS with cookie flags, body-free renew, rotation callback, and idempotent logout covered.

- [ ] **Step 7: Run integration suite for session queries**

Run: `mise run test:integration`

Expected: PASS with generated session queries and rotation transaction.

- [ ] **Step 8: Commit only with explicit authorization**

```bash
git add internal/config internal/api/session_cookie.go internal/api/session_cookie_test.go internal/api/user.go internal/api/user_test.go internal/api/token_renew_test.go internal/api/routes.go internal/api/errors.go compose.yaml
git commit -m "feat(auth): use rotating refresh cookies"
```

---

### Task 5: Require Verification And Hide Email Membership

**Files:**
- Modify: `internal/db/query/users.sql`
- Regenerate: `internal/db/sqlc/users.sql.go`
- Regenerate: `internal/db/sqlc/querier.go`
- Modify: `internal/db/errors.go`
- Modify: `internal/db/errors_test.go`
- Modify: `internal/api/errors.go`
- Modify: `internal/api/user.go`
- Modify: `internal/api/user_test.go`
- Modify: `internal/api/verify_email_test.go`
- Modify: `internal/api/routes.go`
- Modify: `internal/worker/verify_email.go`
- Modify: `internal/worker/verify_email_test.go`
- Create: `internal/worker/registration_notice.go`
- Create: `internal/worker/registration_notice_test.go`
- Modify: `internal/worker/client.go`

**Interfaces:**
- Produces: `ErrUsernameExists` and `ErrEmailExists` from PostgreSQL constraint names.
- Produces: sqlc `GetUserByEmail(ctx, email)`.
- Produces: `SendVerifyEmailArgs.InsertOpts()` unique by username for 15 minutes.
- Produces: `SendRegistrationNoticeArgs.InsertOpts()` unique by email for 1 hour.
- Produces: `POST /api/v1/users/verify_email/resend` with generic `202`.
- Changes registration success from `201` user JSON to generic `202`.

- [ ] **Step 1: Write failing API privacy and verification tests**

Update/add tests that require:

```go
func TestLoginUserUnverified(t *testing.T) {
	t.Parallel()
	hashed, err := password.Hash("secret123")
	if err != nil {
		t.Fatal(err)
	}
	fake := fakeStore{
		getUser: func(context.Context, string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: "alice", HashedPassword: hashed, IsEmailVerified: false}, nil
		},
		createSession: func(context.Context, sqlcdb.CreateSessionParams) (sqlcdb.Session, error) {
			t.Fatal("unverified login must not create a session")
			return sqlcdb.Session{}, nil
		},
	}
	s := newTestServerWithStore(t, fake)
	rec := postLogin(t, s, "alice", "secret123")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d (%s)", rec.Code, rec.Body.String())
	}
}
```

Registration tests must assert that a new user and `ErrEmailExists` both return identical `202` status and body, while `ErrUsernameExists` returns `409`. Resend tests must assert unknown, verified, and unverified email paths all return the same generic `202` response.

- [ ] **Step 2: Run RED unit suite**

Run: `mise run test:unit`

Expected: FAIL because unverified users currently receive tokens and registration returns `201`/generic `409`.

- [ ] **Step 3: Classify username and email constraints**

Add `GetUserByEmail`:

```sql
-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 LIMIT 1;
```

Run: `mise run sqlc:generate`

Extend `ClassifyError` for SQLSTATE `23505`:

```go
switch pgErr.ConstraintName {
case "users_username_key":
	return ErrUsernameExists
case "users_email_key":
	return ErrEmailExists
default:
	return ErrUniqueViolation
}
```

Map `ErrUsernameExists` to `409`; handle `ErrEmailExists` directly in registration so it never reaches the generic catalog.

- [ ] **Step 4: Add bounded River uniqueness and notice worker**

Make verification jobs unique by username for the verification-link lifetime:

```go
type SendVerifyEmailArgs struct {
	Username string `json:"username" river:"unique"`
}

func (SendVerifyEmailArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{
		ByArgs: true, ByPeriod: 15 * time.Minute,
	}}
}
```

Create `registration_notice.go` with fixed, non-sensitive copy and one-hour per-email uniqueness:

```go
type SendRegistrationNoticeArgs struct {
	Email string `json:"email" river:"unique"`
}

func (SendRegistrationNoticeArgs) Kind() string { return "send_registration_notice" }

func (SendRegistrationNoticeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{
		ByArgs: true, ByPeriod: time.Hour,
	}}
}

func (w *SendRegistrationNoticeWorker) Work(ctx context.Context, job *river.Job[SendRegistrationNoticeArgs]) error {
	const body = `A registration attempt used this email address. If this was you, sign in to your existing SimpleBank account. If not, no action is required.`
	return w.mailer.Send(ctx, job.Args.Email, "SimpleBank registration attempt", body)
}
```

Register the worker in `NewClient`. Unit tests must compare `InsertOpts().UniqueOpts` and assert the notice body does not echo attacker-controlled request fields.

- [ ] **Step 5: Implement generic registration and resend**

Use one response constant for both flows:

```go
var verificationAccepted = map[string]string{
	"message": "check your email for verification instructions",
}
```

On successful creation return `202` with that body. On `ErrEmailExists`, enqueue `SendRegistrationNoticeArgs{Email: req.Email}` when the River client is available, log enqueue failure server-side, and still return the same `202`. On `ErrUsernameExists`, return the mapped `409`.

For resend, bind `{email}`, call `GetUserByEmail`, and enqueue `SendVerifyEmailArgs` only for an unverified user. Treat unknown and verified addresses as no-op success. Every branch returns the same `202` body.

After a valid password comparison in login, add:

```go
if !user.IsEmailVerified {
	return echo.NewHTTPError(http.StatusForbidden, "email verification required")
}
```

- [ ] **Step 6: Run GREEN unit suite**

Run: `mise run test:unit`

Expected: PASS for unverified denial, generic email responses, username conflict, resend, and River uniqueness.

- [ ] **Step 7: Run integration suite**

Run: `mise run test:integration`

Expected: PASS with generated email lookup and constraint-specific errors.

- [ ] **Step 8: Commit only with explicit authorization**

```bash
git add internal/db/query/users.sql internal/db/sqlc internal/db/errors.go internal/db/errors_test.go internal/api internal/worker
git commit -m "fix(auth): require verified email login"
```

---

### Task 6: Cap Demo Opening Balances

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/api/account.go`
- Modify: `internal/api/account_test.go`
- Modify: `internal/api/meta.go`
- Modify: `internal/api/meta_test.go`
- Modify: `internal/api/routes.go`
- Modify: `compose.yaml`

**Interfaces:**
- Produces config: `AccountOpeningLimits map[string]int64` from `ACCOUNT_OPENING_LIMITS` / `--account-opening-limits`.
- Produces: `OpeningBalanceLimitFor(currency string) int64`.
- Produces: `GET /api/v1/account-opening-limits` returning `{}` rather than `null` when empty.
- Changes account creation: balance above cap returns `422` before store access.

- [ ] **Step 1: Write failing boundary tests**

Add config parsing tests for empty, valid, malformed, and negative values. Add table-driven handler tests for below, equal, and above cap:

```go
func TestCreateAccountOpeningBalanceLimit(t *testing.T) {
	tests := []struct {
		name string
		balance int64
		wantStatus int
		wantStore bool
	}{
		{name: "below", balance: 99999, wantStatus: http.StatusCreated, wantStore: true},
		{name: "equal", balance: 100000, wantStatus: http.StatusCreated, wantStore: true},
		{name: "above", balance: 100001, wantStatus: http.StatusUnprocessableEntity, wantStore: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			fake := fakeStore{createAccountTx: func(_ context.Context, arg sqlcdb.CreateAccountParams) (sqlcdb.Account, error) {
				called = true
				return sqlcdb.Account{ID: uuid.New(), Owner: arg.Owner, Balance: arg.Balance, Currency: arg.Currency}, nil
			}}
			s := newTestServerWithConfig(t, fake, config.Config{
				JWTSecret: testSecret, AccessTTL: time.Minute, RefreshTTL: time.Hour,
				AccountOpeningLimits: map[string]int64{"USD": 100000},
			})
			body := fmt.Sprintf(`{"currency":"USD","balance":%d}`, tt.balance)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", bearer(t, "alice"))
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus || called != tt.wantStore {
				t.Fatalf("status=%d called=%v", rec.Code, called)
			}
		})
	}
}
```

Also assert a missing currency entry permits zero and rejects positive opening balance.

- [ ] **Step 2: Run RED unit suite**

Run: `mise run test:unit`

Expected: FAIL because opening-limit config and endpoint do not exist and over-cap balances reach the store.

- [ ] **Step 3: Implement opening-limit config**

Parse a `map[string]int64`, preserve `nil` for empty input, reject malformed JSON and any negative cap in `Validate`, and expose:

```go
func (c Config) OpeningBalanceLimitFor(currencyCode string) int64 {
	return c.AccountOpeningLimits[currencyCode]
}
```

Add a `cli.StringFlag` named `account-opening-limits` sourced from `ACCOUNT_OPENING_LIMITS`.

- [ ] **Step 4: Enforce and publish policy**

After supported-currency validation and before authentication/store access:

```go
if req.Balance > s.config.OpeningBalanceLimitFor(req.Currency) {
	return echo.NewHTTPError(
		http.StatusUnprocessableEntity,
		"opening balance exceeds the configured limit",
	)
}
```

Add a public handler mirroring `transferLimits`, returning an empty object for nil. Register `GET /account-opening-limits`.

Add this compose anchor and assign it to every app profile:

```yaml
x-account-opening-limits: &account-opening-limits '{"USD":100000,"EUR":100000,"VND":25000000}'
```

- [ ] **Step 5: Run GREEN unit suite**

Run: `mise run test:unit`

Expected: PASS across config, endpoint, and account boundary cases.

- [ ] **Step 6: Commit only with explicit authorization**

```bash
git add internal/config internal/api/account.go internal/api/account_test.go internal/api/meta.go internal/api/meta_test.go internal/api/routes.go compose.yaml
git commit -m "feat(accounts): cap demo opening balances"
```

---

### Task 7: Migrate SPA To Cookie Sessions And Generic Registration

**Files:**
- Modify: `frontend/src/lib/api/types.ts`
- Modify: `frontend/src/lib/api/client.ts`
- Modify: `frontend/src/lib/api/client.test.ts`
- Modify: `frontend/src/lib/stores/auth.svelte.ts`
- Create: `frontend/src/lib/stores/auth.svelte.test.ts`
- Modify: `frontend/src/lib/components/AppHeader.svelte`
- Modify: `frontend/src/lib/pages/RegisterPage.svelte`
- Modify: `frontend/src/lib/pages/LoginPage.svelte`

**Interfaces:**
- Changes `LoginResponse`: no refresh fields.
- Changes `RenewResponse`: access fields plus `user`.
- Produces `AcceptedResponse { message: string }`.
- Changes `AuthStore.init()` to restore from cookie by calling renew unconditionally.
- Changes `AuthStore.logout()` to return `Promise<void>` and always clear local state.

- [ ] **Step 1: Write failing frontend session tests**

Update `client.test.ts` so every authenticated `401` attempts one refresh without consulting `canRefresh`. Add `auth.svelte.test.ts` with isolated stores (export `AuthStore` for tests):

```ts
it("removes the legacy refresh-token record and restores from the cookie", async () => {
  localStorage.setItem("simplebank.session", JSON.stringify({ refreshToken: "stolen" }));
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      jsonResponse(200, {
        access_token: "new-access",
        access_token_expires_at: new Date(Date.now() + 60_000).toISOString(),
        user: verifiedUser,
      }),
    ),
  );
  const store = new AuthStore();

  await store.init();

  expect(localStorage.getItem("simplebank.session")).toBeNull();
  expect(store.accessToken).toBe("new-access");
  expect(store.user).toEqual(verifiedUser);
});

it("clears local state even when server logout fails", async () => {
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
  const store = new AuthStore();
  store.user = verifiedUser;
  store.accessToken = "access";

  await store.logout();

  expect(store.user).toBeNull();
  expect(store.accessToken).toBeNull();
});
```

- [ ] **Step 2: Run RED frontend suite**

Run: `mise run frontend:test`

Expected: FAIL because auth still reads/writes refresh tokens in localStorage and logout is client-only.

- [ ] **Step 3: Update API client and types**

Set fetch credentials explicitly:

```ts
return fetch(`${BASE_URL}${path}`, {
  method: options.method ?? "GET",
  headers,
  body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  credentials: "same-origin",
  signal: options.signal,
});
```

Remove refresh fields from `LoginResponse`; add `user: User` to `RenewResponse`; add `AcceptedResponse`. On an authenticated `401`, call `auth.tryRefresh()` once without a browser-readable `canRefresh` flag.

- [ ] **Step 4: Replace browser token persistence**

Delete `PersistedSession`, `loadSession`, `saveSession`, and `#refreshToken`. Implement:

```ts
export class AuthStore {
  user = $state<User | null>(null);
  accessToken = $state<string | null>(null);
  initializing = $state(true);

  get isAuthenticated(): boolean {
    return this.user !== null && this.accessToken !== null;
  }

  async init(): Promise<void> {
    try {
      localStorage.removeItem("simplebank.session");
    } catch {
      // Storage can be unavailable; cookie restoration still works.
    }
    await this.tryRefresh();
    this.initializing = false;
  }

  async tryRefresh(): Promise<boolean> {
    try {
      const response = await request<RenewResponse>("/tokens/renew", { method: "POST" });
      this.accessToken = response.access_token;
      this.user = response.user;
      return true;
    } catch {
      this.clear();
      return false;
    }
  }

  async logout(): Promise<void> {
    try {
      await request<void>("/users/logout", { method: "POST" });
		} catch {
			// Local sign-out must complete even when revocation cannot reach server.
    } finally {
      this.clear();
      navigate("/login");
    }
  }
}
```

Login sets only access token and returned user. Registration returns `AcceptedResponse`. In `AppHeader`, call `void auth.logout()` after resetting accounts.

- [ ] **Step 5: Keep registration and verification copy generic**

`RegisterPage` continues navigating to login after `202`, but does not consume user data. Keep login's success alert as `Account request accepted. Check your email to verify it, then sign in.` so it is true for both new and existing-email requests.

- [ ] **Step 6: Run GREEN frontend suite**

Run: `mise run frontend:test`

Expected: PASS; no test or production path writes a refresh token to localStorage.

- [ ] **Step 7: Run frontend type and lint gates**

Run: `mise run frontend:check`

Expected: PASS.

Run: `mise run frontend:lint`

Expected: PASS.

- [ ] **Step 8: Commit only with explicit authorization**

```bash
git add frontend/src/lib/api frontend/src/lib/stores/auth.svelte.ts frontend/src/lib/stores/auth.svelte.test.ts frontend/src/lib/components/AppHeader.svelte frontend/src/lib/pages/RegisterPage.svelte frontend/src/lib/pages/LoginPage.svelte
git commit -m "feat(frontend): use cookie-backed sessions"
```

---

### Task 8: Add Opening-Limit UX, Documentation, And Final Gates

**Files:**
- Create: `frontend/src/lib/opening-limits.ts`
- Create: `frontend/src/lib/opening-limits.test.ts`
- Modify: `frontend/src/lib/api/types.ts`
- Modify: `frontend/src/lib/pages/NewAccountPage.svelte`
- Modify: `README.md`
- Modify: `docs/decisions/0005-transfer-safety-idempotency-and-limits.md`

**Interfaces:**
- Produces frontend type: `AccountOpeningLimits = Partial<Record<Currency, number>>`.
- Produces: `openingLimitFor(limits, currency)`, `openingLimitInputMax(limit, currency)`, and `validateOpeningBalance(balance, currency, limits)`.
- Consumes: public `GET /account-opening-limits`.

- [ ] **Step 1: Write failing pure frontend tests**

Create `opening-limits.test.ts`:

```ts
describe("opening limits", () => {
  const limits = { USD: 100000, EUR: 100000, VND: 25000000 };

  it("treats a missing currency as a zero cap", () => {
    expect(openingLimitFor({}, "USD")).toBe(0);
  });

  it("accepts the boundary and rejects one minor unit above it", () => {
    expect(validateOpeningBalance(100000, "USD", limits)).toBeNull();
    expect(validateOpeningBalance(100001, "USD", limits)).toContain("maximum");
  });

  it("formats input max without floating-point rounding", () => {
    expect(openingLimitInputMax(100000, "USD")).toBe("1000.00");
    expect(openingLimitInputMax(25000000, "VND")).toBe("25000000");
  });
});
```

- [ ] **Step 2: Run RED frontend suite**

Run: `mise run frontend:test`

Expected: FAIL because opening-limit helpers do not exist.

- [ ] **Step 3: Implement pure opening-limit helpers**

Use integer string formatting, not floating multiplication:

```ts
export type AccountOpeningLimits = Partial<Record<Currency, number>>;

export function openingLimitFor(limits: AccountOpeningLimits, currency: Currency): number {
  return limits[currency] ?? 0;
}

export function openingLimitInputMax(limit: number, currency: Currency): string {
  const digits = fractionDigits(currency);
  if (digits === 0) return String(limit);
  const raw = String(limit).padStart(digits + 1, "0");
  return `${raw.slice(0, -digits)}.${raw.slice(-digits)}`;
}

export function validateOpeningBalance(
  balance: number,
  currency: Currency,
  limits: AccountOpeningLimits,
): string | null {
  const limit = openingLimitFor(limits, currency);
  return balance <= limit
    ? null
    : `Opening deposit exceeds the ${formatMoney(limit, currency)} maximum.`;
}
```

- [ ] **Step 4: Load and enforce policy in account form**

On mount, fetch `AccountOpeningLimits` from `/account-opening-limits` while accounts load. Keep the form disabled until policy fetch completes; on fetch failure show an error and permit retry rather than assuming an unlimited cap.

After parsing the deposit and before submit:

```ts
const limitMessage = validateOpeningBalance(balance, currency, openingLimits);
if (limitMessage) {
  depositError = limitMessage;
  return;
}
```

Set the numeric input's `max` to `openingLimitInputMax(openingLimitFor(openingLimits, currency), currency)` and change hint copy to show `Maximum ${formatMoney(limit, currency)}.`. Server validation remains authoritative.

- [ ] **Step 5: Run GREEN frontend tests and checks**

Run: `mise run frontend:test`

Expected: PASS.

Run: `mise run frontend:check`

Expected: PASS.

Run: `mise run frontend:lint`

Expected: PASS.

- [ ] **Step 6: Update public documentation and ADR**

Update `README.md` configuration table with:

```markdown
| `SESSION_COOKIE_SECURE` | `--session-cookie-secure` | `true` | Require HTTPS for refresh cookie transport |
| `ACCOUNT_OPENING_LIMITS` | `--account-opening-limits` | — | Per-currency demo opening caps in minor units |
```

Update API rows: registration `202`, cookie-based renew, logout, resend, and opening-limit policy endpoint. State that protected routes accept access tokens only and refresh tokens are HttpOnly cookies.

Amend ADR-0005 idempotency section from global uniqueness to `(from_account_id, idempotency_key)` and document `409` on mismatched payload replay.

- [ ] **Step 7: Run complete repository validation**

Run in order:

```bash
mise run test:unit
mise run test:integration
mise run frontend:test
mise run frontend:check
mise run frontend:lint
mise run golangci-lint
mise run app:build
mise run govulncheck
```

Expected: every task exits `0`; govulncheck reports zero reachable vulnerabilities.

- [ ] **Step 8: Inspect security invariants in final diff**

Run:

```bash
git diff --check
git status --short
```

Then search the diff and source to confirm:

- no refresh token JSON field remains;
- no auth credential is written to `localStorage` or `sessionStorage`;
- every `CreateToken` and `VerifyToken` call names a token purpose;
- protected middleware expects `token.Access`;
- renewal and logout expect `token.Refresh`;
- idempotency lookup includes source account ID;
- opening caps are checked on server before account creation.

- [ ] **Step 9: Commit only with explicit authorization**

```bash
git add frontend/src/lib/opening-limits.ts frontend/src/lib/opening-limits.test.ts frontend/src/lib/api/types.ts frontend/src/lib/pages/NewAccountPage.svelte README.md docs/decisions/0005-transfer-safety-idempotency-and-limits.md docs/superpowers/specs/2026-08-11-authentication-authorization-hardening-design.md docs/superpowers/plans/2026-08-11-authentication-authorization-hardening.md
git commit -m "feat: complete auth flow hardening"
```
