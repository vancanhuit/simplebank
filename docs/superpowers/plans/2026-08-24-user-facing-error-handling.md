# User-Facing Error Handling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver safe, stable API errors and calm, actionable SPA recovery without losing user work or misclassifying temporary failures as expired sessions.

**Architecture:** The Go API owns stable error codes and client-safe fallback text. The frontend API client classifies failures and owns generic copy, while auth owns refresh outcomes and pages add operation context and retry controls; the existing transfer idempotency, optimistic notification rollback, and session-generation guards remain authoritative.

**Tech Stack:** Go 1.27, Echo v5, Svelte 5.56, TypeScript 6, daisyUI 5, Vitest, Testing Library, Playwright, `mise`.

**Spec:** `docs/superpowers/specs/2026-08-24-user-facing-error-handling-design.md`

## Global Constraints

- Keep money as `int64` minor units and do not alter transfer authorization, source-scoped idempotency, deterministic locking, limit validation, or guarded balance updates.
- Preserve auth and account reset generation checks so stale responses cannot repopulate another session.
- Never expose arbitrary backend, browser, JSON parser, database, token, or validation messages to users.
- Do not automatically retry money-moving requests.
- Do not display request IDs or internal diagnostics.
- Use the shared `Alert` component for inline UI errors and preserve accessible live-region behavior.
- Add no dependency, telemetry service, localization framework, offline/PWA support, or unrelated visual redesign.
- Use `apply_patch` for manual edits. Use the Svelte MCP documentation and run the Svelte autofixer until clean for every changed `.svelte` file.
- Before every commit, inspect `git status`, `git diff`, and `git log --oneline -10`; stage only task files and preserve unrelated worktree changes.

---

## File Structure

**Backend contract**

- `internal/api/errors.go`: defines the JSON response, typed explicit HTTP error, domain catalog, framework fallback mapping, and central response writer.
- `internal/api/errors_test.go`: verifies status/code/message triples and redaction.
- `internal/api/{validator,middleware,user,account,transfer,notification}.go`: assigns stable codes at each explicit trust-boundary or domain branch.
- `internal/api/{account,auth,notification,rate_limit,token_renew,transfer,user,verify_email}_test.go`: asserts representative endpoint codes while preserving statuses.

**Frontend classification and session lifecycle**

- `frontend/src/lib/api/client.ts`: defines `ApiErrorKind`, `ApiError`, safe decoding/classification, retry metadata, refresh coalescing, and `toMessage`.
- `frontend/src/lib/api/client.test.ts`: proves raw text cannot cross the UI boundary and verifies refresh coalescing/classification.
- `frontend/src/lib/stores/auth.svelte.ts`: defines explicit refresh outcomes and transient renewal state.
- `frontend/src/lib/stores/auth.svelte.test.ts`: proves `200`/`204`/`401`/network/`5xx` semantics and generation safety.
- `frontend/src/lib/router.svelte.ts`: validates internal return paths.
- `frontend/src/lib/router.svelte.test.ts`: rejects external or public return targets.

**Frontend presentation**

- `frontend/src/App.svelte`: owns startup recovery, persistent renewal alert, definitive session-expiry navigation, replace-style guards, and return path capture.
- `frontend/src/App.test.ts`: exercises the app-level auth and recovery transitions.
- `frontend/src/lib/pages/LoginPage.svelte`: consumes one-shot expiry state and validated return path.
- `frontend/src/lib/pages/LoginPage.test.ts`: verifies notices and return navigation.
- `frontend/src/lib/{pages,components}/*.svelte`: replaces raw errors with operation-specific safe messages and shared accessible alerts.
- Existing component/page tests: assert copy, recovery actions, preserved data, and retry behavior.
- `frontend/src/lib/components/AppErrorFallback.svelte`: renders the sanitized root fallback.
- `frontend/src/lib/components/AppErrorFallback.test.ts`: verifies no thrown text is rendered and both recovery actions work.
- `frontend/src/main.ts`: places the root `svelte:boundary` around `App` through a small root component.
- `frontend/src/Root.svelte`: owns the root boundary and development-only console reporting.
- `frontend/src/Root.test.ts`: forces render failure and verifies boundary recovery.
- `frontend/e2e/{accessibility,rate-limit}.spec.ts`: validates representative accessible recovery and additive API-code mocks.

---

### Task 1: Structured API Error Contract

**Files:**
- Modify: `internal/api/errors.go:13-68`
- Modify: `internal/api/errors_test.go:16-97`
- Test: `internal/api/errors_test.go`

**Interfaces:**
- Produces: `type errorResponse struct { Code string; Error string }`
- Produces: `type apiError struct { status int; code string; message string }`
- Produces: `func newAPIError(status int, code, message string) error`
- Produces: `func lookupError(error) apiError`
- Produces: stable JSON `{ "code": string, "error": string }` for every API failure.

Use this exact domain catalog:

| Domain error | Status | Code | Safe fallback |
|---|---:|---|---|
| `store.ErrRecordNotFound` | 404 | `not_found` | `resource not found` |
| `store.ErrUsernameExists` | 409 | `username_exists` | `username already exists` |
| `store.ErrEmailExists` | 409 | `email_exists` | `email already exists` |
| `store.ErrUniqueViolation` | 409 | `already_exists` | `resource already exists` |
| `store.ErrForeignKeyViolation` | 409 | `related_not_found` | `related resource not found` |
| `store.ErrInsufficientBalance` | 422 | `insufficient_balance` | `insufficient balance` |
| `store.ErrBalanceLimitExceeded` | 422 | `destination_balance_limit_exceeded` | `destination balance exceeds the supported limit` |
| `store.ErrCurrencyMismatch` | 400 | `currency_mismatch` | `currency mismatch` |
| `store.ErrDailyLimitExceeded` | 422 | `daily_limit_exceeded` | `daily transfer limit exceeded` |
| `store.ErrNumericOutOfRange` | 422 | `amount_too_large` | `amount too large` |
| `store.ErrIdempotencyConflict` | 409 | `idempotency_conflict` | `idempotency key conflicts with an existing transfer` |
| `store.ErrInvalidSession` | 401 | `invalid_session` | `invalid session` |
| `token.ErrExpiredToken` | 401 | `token_expired` | `token has expired` |
| `token.ErrInvalidToken` | 401 | `token_invalid` | `token is invalid` |

- [ ] **Step 1: Replace message-only tests with status/code/message contract tests**

Add table entries for every existing domain sentinel, including the currently omitted `ErrEmailExists`, `ErrCurrencyMismatch`, `ErrDailyLimitExceeded`, and `ErrNumericOutOfRange`. Use this shape:

```go
tests := []struct {
	name        string
	err         error
	wantStatus  int
	wantCode    string
	wantMessage string
}{
	{"not found", store.ErrRecordNotFound, http.StatusNotFound, "not_found", "resource not found"},
	{"username exists", store.ErrUsernameExists, http.StatusConflict, "username_exists", "username already exists"},
	{"email exists", store.ErrEmailExists, http.StatusConflict, "email_exists", "email already exists"},
	{"insufficient balance", store.ErrInsufficientBalance, http.StatusUnprocessableEntity, "insufficient_balance", "insufficient balance"},
	{"unknown", errors.New("database password leaked"), http.StatusInternalServerError, "internal_error", "internal server error"},
}
```

Update the handler tests to decode `errorResponse`. Add cases for `newAPIError(http.StatusBadRequest, "invalid_account_id", "invalid account id")`, `echo.ErrMethodNotAllowed`, and an unrestricted `echo.NewHTTPError(http.StatusTeapot, "sensitive detail")`; the unrestricted error must preserve the status but return code `request_failed` and `http.StatusText(http.StatusTeapot)` rather than the custom text.

- [ ] **Step 2: Run the focused tests and verify the contract fails**

Run: `go test -race ./internal/api -run '^(TestLookupError|TestErrorHandlerPreservesErrorSemantics)$'`

Expected: FAIL because `lookupError` has no code and `errorHandler` still emits only `error` while exposing custom `HTTPError.Message`.

- [ ] **Step 3: Implement the central structured error model**

Implement these minimal types and rules in `errors.go`:

```go
type errorResponse struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

type apiError struct {
	status  int
	code    string
	message string
}

func (e apiError) Error() string { return e.message }

func newAPIError(status int, code, message string) error {
	return apiError{status: status, code: code, message: message}
}
```

Change each catalog row to carry `code`, return an `apiError`, and make the unknown catalog result exactly `500/internal_error/internal server error`. In `errorHandler`, resolve in this order: committed response, typed `apiError`, domain catalog, Echo status. For a generic Echo status use a small status-to-code switch for `400`, `401`, `403`, `404`, `405`, `409`, `413`, `415`, and `429`, with `request_failed` as the default; always use `http.StatusText(status)` as its safe message.

- [ ] **Step 4: Run the focused API error tests**

Run: `go test -race ./internal/api -run '^(TestLookupError|TestErrorHandlerPreservesErrorSemantics)$'`

Expected: PASS; unrestricted custom text is absent from every response body.

- [ ] **Step 5: Format and commit the contract**

Run: `mise run golangci-lint:fmt`

Commit:

```bash
git add internal/api/errors.go internal/api/errors_test.go
git commit -m "feat(api): add structured error responses"
```

---

### Task 2: Explicit Codes At API Trust Boundaries

**Files:**
- Modify: `internal/api/validator.go:15-41`
- Modify: `internal/api/middleware.go:31-85`
- Modify: `internal/api/user.go:147-168,241-252,321-340`
- Modify: `internal/api/account.go:23-90`
- Modify: `internal/api/transfer.go:29-54,92-105,148-155`
- Modify: `internal/api/notification.go:102-120,159-163,234-236`
- Modify: `internal/api/routes.go:17-34`
- Modify: `internal/api/account_test.go`
- Modify: `internal/api/auth_test.go`
- Modify: `internal/api/notification_test.go`
- Modify: `internal/api/rate_limit_test.go:43-49`
- Modify: `internal/api/token_renew_test.go`
- Modify: `internal/api/transfer_test.go`
- Modify: `internal/api/user_test.go`
- Modify: `internal/api/verify_email_test.go`
- Test: `internal/api/*_test.go`

**Interfaces:**
- Consumes: `newAPIError(status int, code, message string) error` from Task 1.
- Produces: stable explicit codes listed below without changing existing HTTP statuses.

| Condition | Code |
|---|---|
| malformed JSON/body binding | `invalid_request_body` |
| failed payload rules | `invalid_request_payload` |
| bad credentials | `invalid_credentials` |
| unverified login | `email_verification_required` |
| missing/invalid ownership | `forbidden` |
| rejected browser origin | `cross_origin_denied` |
| unsupported currency | `unsupported_currency` |
| opening balance over safe/configured cap | `opening_balance_limit_exceeded` |
| transfer over safe cap | `amount_too_large` |
| per-transfer cap | `transfer_limit_exceeded` |
| same source/destination | `same_account_transfer` |
| bad account UUID | `invalid_account_id` |
| bad notification UUID/cursor | `invalid_notification_id` / `invalid_notification_cursor` |
| bad pagination | `invalid_page` / `invalid_size` |
| bad verification ID/link | `invalid_verification_link` |
| credential middleware `429` | `rate_limited` |

- [ ] **Step 1: Add endpoint assertions for representative explicit codes**

Add or update assertions at the behavior-owning tests. Decode `errorResponse` rather than comparing exact JSON strings. At minimum prove:

```go
if got.Code != "opening_balance_limit_exceeded" {
	t.Fatalf("code = %q, want opening_balance_limit_exceeded", got.Code)
}
```

Cover one test for every row in the table; reuse existing failing endpoint cases rather than creating duplicate setup. In `rate_limit_test.go`, assert both `body["code"] == "rate_limited"` and the existing `Retry-After` header.

- [ ] **Step 2: Run affected package tests and verify explicit codes fail**

Run: `go test -race ./internal/api`

Expected: FAIL because explicit handler branches still become generic framework codes.

- [ ] **Step 3: Replace free-form handler errors with `newAPIError`**

Convert all JSON API `echo.NewHTTPError` calls in the listed files to the exact code table. Keep `spa.go` unchanged because its HTML/asset 404 behavior is not an API error contract. Keep Echo middleware errors such as `echo.ErrUnauthorized`; the central fallback safely maps them.

Ensure `ErrEmailExists` has its distinct catalog mapping. Preserve login anti-enumeration: unknown user and wrong password must both remain the same `401/invalid_credentials` response and still execute the dummy hash comparison.

- [ ] **Step 4: Make the rate limiter use the central contract**

Set `DenyHandler` on both `credentialLimiter` and `authLimiter` in `internal/api/routes.go`:

```go
DenyHandler: func(_ *echo.Context, _ string, _ error) error {
	return newAPIError(http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")
},
```

Do not remove or alter `Retry-After`, `X-RateLimit-Limit`, per-client isolation, or per-endpoint isolation.

- [ ] **Step 5: Run and format the API package**

Run: `go test -race ./internal/api`

Expected: PASS.

Run: `mise run golangci-lint:fmt`

- [ ] **Step 6: Commit explicit API codes**

```bash
git add internal/api/validator.go internal/api/middleware.go internal/api/user.go internal/api/account.go internal/api/transfer.go internal/api/notification.go internal/api/routes.go internal/api/account_test.go internal/api/auth_test.go internal/api/notification_test.go internal/api/rate_limit_test.go internal/api/token_renew_test.go internal/api/transfer_test.go internal/api/user_test.go internal/api/verify_email_test.go
git commit -m "feat(api): classify client-safe errors"
```

---

### Task 3: Safe Frontend Error Classification

**Files:**
- Modify: `frontend/src/lib/api/client.ts:7-148`
- Modify: `frontend/src/lib/api/client.test.ts:1-293`
- Test: `frontend/src/lib/api/client.test.ts`

**Interfaces:**
- Produces: `type ApiErrorKind = "api" | "network" | "invalid_response" | "aborted" | "session_unavailable"`
- Produces: `class ApiError` with `kind`, `status`, `code`, and `retryAfterSeconds`.
- Produces: `function toMessage(error: unknown): string` that never returns arbitrary thrown text.
- Produces: `function isRetryable(error: unknown): boolean` so pages do not duplicate status/code switches.
- Consumes: backend `{ code, error }` responses from Tasks 1-2.

- [ ] **Step 1: Write failing classification and redaction tests**

Replace the raw-message test with code-aware assertions and add these cases:

```typescript
await expect(request("/transfers", { method: "POST", body: {} })).rejects.toMatchObject({
  kind: "api",
  status: 422,
  code: "insufficient_balance",
});
```

Add tests proving:

- `{ code: "insufficient_balance", error: "database password leaked" }` formats as `"You don't have enough money in this account."`.
- unknown code plus arbitrary server text formats by status, not server text.
- a rejected `fetch(new TypeError("Failed to fetch secret host"))` becomes kind `network` and the message `"We couldn't reach SimpleBank. Check your connection and try again."`.
- an `AbortError` becomes kind `aborted` and does not expose its message.
- invalid JSON on `200` becomes kind `invalid_response` and formats as `"SimpleBank returned an unexpected response. Please try again."`.
- `503` formats as `"SimpleBank is temporarily unavailable. Please try again."`.
- `429` stores `retryAfterSeconds: 5` and preserves `"Too many attempts. Try again in 5 seconds."`.
- `toMessage(new Error("private path"))` returns the generic fallback.
- `isRetryable` returns true for network, invalid-response, session-unavailable, `408`, `429`, and `5xx` failures; it returns false for aborts and other `4xx` failures.

- [ ] **Step 2: Run the client tests and verify they fail**

Run: `mise run frontend:test -- src/lib/api/client.test.ts`

Expected: FAIL because the current client passes through API/native messages and has no failure kinds or codes.

- [ ] **Step 3: Implement `ApiError` classification**

Use one class rather than parallel subclasses:

```typescript
export type ApiErrorKind = "api" | "network" | "invalid_response" | "aborted" | "session_unavailable";

export class ApiError extends Error {
  constructor(
    readonly kind: ApiErrorKind,
    readonly status: number | null = null,
    readonly code: string | null = null,
    readonly retryAfterSeconds: number | null = null,
  ) {
    super(code ?? kind);
    this.name = "ApiError";
  }
}
```

Wrap `fetch` in `send`: classify `AbortError` as `aborted` and every other rejection as `network`. In `decode`, parse only `{ code?: unknown; error?: unknown }`; retain a recognized non-empty code, but do not put server `error` text in `ApiError.message`. Convert malformed successful JSON to `invalid_response`. Keep no-content success returning `undefined`.

Implement `toMessage` as a code-first switch, then kind/status fallback. Include at least these domain mappings: `invalid_credentials`, `email_verification_required`, `username_exists`, `email_exists`, `insufficient_balance`, `destination_balance_limit_exceeded`, `currency_mismatch`, `daily_limit_exceeded`, `transfer_limit_exceeded`, `same_account_transfer`, `idempotency_conflict`, `invalid_verification_link`, `not_found`, and `forbidden`. Implement `isRetryable` beside it from the classified fields, not from message text.

- [ ] **Step 4: Adapt refresh-coalescing tests to explicit outcomes temporarily**

Until Task 4 changes auth, keep `tryRefresh(): Promise<boolean>` in production. Update test response fixtures to include `code`, and preserve existing proofs: one refresh per generation, no retry after generation change, concurrent sharing, and a new refresh for later cycles.

- [ ] **Step 5: Run the focused frontend tests**

Run: `mise run frontend:test -- src/lib/api/client.test.ts`

Expected: PASS.

Run: `mise run frontend:check`

Expected: PASS.

- [ ] **Step 6: Commit safe frontend classification**

```bash
git add frontend/src/lib/api/client.ts frontend/src/lib/api/client.test.ts
git commit -m "feat(frontend): classify API failures safely"
```

---

### Task 4: Refresh Outcomes And Transient Session Recovery

**Files:**
- Modify: `frontend/src/lib/stores/auth.svelte.ts:15-125`
- Modify: `frontend/src/lib/stores/auth.svelte.test.ts:22-287`
- Modify: `frontend/src/lib/api/client.ts:7-99`
- Modify: `frontend/src/lib/api/client.test.ts:62-210,238-259`
- Test: `frontend/src/lib/stores/auth.svelte.test.ts`
- Test: `frontend/src/lib/api/client.test.ts`

**Interfaces:**
- Produces: `type RefreshOutcome = "refreshed" | "no_session" | "expired" | "unavailable" | "stale"`
- Produces: `auth.renewalUnavailable: boolean`
- Produces: `auth.sessionExpired: boolean` and `auth.consumeSessionExpired(): boolean`
- Produces: `auth.retryRefresh(): Promise<RefreshOutcome>`
- Changes: `auth.tryRefresh(): Promise<RefreshOutcome>`
- Consumes: `ApiError.kind`, `ApiError.status`, and `ApiError.code` from Task 3.

- [ ] **Step 1: Write failing auth outcome tests**

Update existing boolean expectations and add exact state assertions:

```typescript
expect(await store.tryRefresh()).toBe("refreshed");
expect(await store.tryRefresh()).toBe("no_session"); // 204
expect(await store.tryRefresh()).toBe("expired"); // 401
expect(await store.tryRefresh()).toBe("unavailable"); // network or 503
```

For network and `503`, seed `user` and `accessToken`, then assert both remain unchanged, generation does not advance, and `renewalUnavailable` becomes true. For `204` and `401`, assert state clears and generation advances once; only `401` sets `sessionExpired`. Assert `consumeSessionExpired()` returns true once and then false. For a response completing after logout, assert `"stale"` and no restoration. Add a retry test where the first request rejects and the second returns `200`; `renewalUnavailable` must clear.

- [ ] **Step 2: Run auth tests and verify they fail**

Run: `mise run frontend:test -- src/lib/stores/auth.svelte.test.ts`

Expected: FAIL because all refresh failures currently clear auth and return boolean.

- [ ] **Step 3: Implement explicit outcomes in `AuthStore`**

Add:

```typescript
export type RefreshOutcome = "refreshed" | "no_session" | "expired" | "unavailable" | "stale";
renewalUnavailable = $state(false);
sessionExpired = $state(false);
```

Use `request<RenewResponse | undefined>` and classify results:

- response data: apply only for current generation, clear `renewalUnavailable`, return `refreshed`.
- `undefined` (`204`): clear auth only for the current generation, leave `sessionExpired` false, and return `no_session`.
- `ApiError` status `401`: clear auth only for the current generation, set `sessionExpired`, and return `expired`.
- network, `5xx`, `invalid_response`: retain state, set `renewalUnavailable`, return `unavailable`.
- changed generation: return `stale` without touching state.

`retryRefresh` delegates to `tryRefresh`. `consumeSessionExpired()` returns the current flag and resets it. Ensure `login`, `logout`, and public `clear()` reset both flags; use a private invalidation helper inside the `401` branch so setting `sessionExpired` is not immediately erased.

- [ ] **Step 4: Update authenticated-request refresh handling**

Change `RefreshAttempt.promise` to `Promise<RefreshOutcome>`. Retry the original request only for `refreshed`. For `unavailable`, throw `new ApiError("session_unavailable")` using the Task 3 constructor defaults instead of re-emitting the original access-token `401`. For `no_session`, `expired`, or `stale`, decode the original response normally. Preserve one coalesced refresh per auth generation.

- [ ] **Step 5: Run auth and client race tests**

Run: `mise run frontend:test -- src/lib/stores/auth.svelte.test.ts src/lib/api/client.test.ts`

Expected: PASS, including coalescing and late-response generation tests.

Run: `mise run frontend:check`

Expected: PASS.

- [ ] **Step 6: Commit refresh lifecycle behavior**

```bash
git add frontend/src/lib/stores/auth.svelte.ts frontend/src/lib/stores/auth.svelte.test.ts frontend/src/lib/api/client.ts frontend/src/lib/api/client.test.ts
git commit -m "fix(frontend): preserve sessions on transient renewal failures"
```

---

### Task 5: Startup Recovery, Session Expiry, And Return Paths

**Files:**
- Modify: `frontend/src/lib/router.svelte.ts:8-51`
- Modify: `frontend/src/lib/router.svelte.test.ts:1-44`
- Modify: `frontend/src/App.svelte:1-155`
- Modify: `frontend/src/App.test.ts:55-257`
- Modify: `frontend/src/lib/pages/LoginPage.svelte:1-124`
- Modify: `frontend/src/lib/pages/LoginPage.test.ts`
- Test: `frontend/src/App.test.ts`
- Test: `frontend/src/lib/pages/LoginPage.test.ts`
- Test: `frontend/src/lib/router.svelte.test.ts`

**Interfaces:**
- Produces: `function safeReturnPath(value: unknown): string | null`
- Consumes: `auth.renewalUnavailable`, `auth.retryRefresh()`, and `RefreshOutcome` from Task 4.
- Uses history state `{ returnTo?: string, sessionExpired?: true }` only after validation.

- [ ] **Step 1: Write failing return-path validation tests**

Add direct cases:

```typescript
expect(safeReturnPath("/transfer")).toBe("/transfer");
expect(safeReturnPath("/accounts/abc?tab=activity")).toBe("/accounts/abc?tab=activity");
expect(safeReturnPath("https://evil.example")).toBeNull();
expect(safeReturnPath("//evil.example")).toBeNull();
expect(safeReturnPath("/login")).toBeNull();
expect(safeReturnPath("/register")).toBeNull();
expect(safeReturnPath(42)).toBeNull();
```

The helper accepts only same-origin absolute paths beginning with one `/`, rejects control characters and public auth routes, and preserves query strings and fragments. Add a navigation test proving `navigate("/accounts/abc?tab=activity#latest")` writes the full URL but keeps `router.path === "/accounts/abc"` so route matching never includes query or fragment text.

- [ ] **Step 2: Write failing app/session recovery tests**

In `App.test.ts`, mock `auth.init` through real state transitions and prove:

- initial network/`503` refresh failure renders `"We couldn't restore your session."` and a `Retry` button, not login.
- retry `200` renders the originally requested protected page.
- authenticated transient renewal failure keeps the current page and cached account/notification stores, and shows one persistent `role="alert"` with retry.
- definitive `401` clears session stores, replaces `/transfer` with `/login`, and state equals `{ returnTo: "/transfer", sessionExpired: true }`.
- signed-out route guard also replaces rather than pushes.

In `LoginPage.test.ts`, prove the expiry notice is consumed once and successful login navigates to validated `returnTo`; malicious return state falls back to `/`.

- [ ] **Step 3: Run the focused tests and verify they fail**

Run: `mise run frontend:test -- src/lib/router.svelte.test.ts src/App.test.ts src/lib/pages/LoginPage.test.ts`

Expected: FAIL because there is no validated return path or transient startup state and guards push history.

- [ ] **Step 4: Implement `safeReturnPath`**

Export it from `router.svelte.ts`. Parse with `new URL(value, window.location.origin)`, require `url.origin === window.location.origin`, require the original input to begin with `/` but not `//`, reject control characters plus `/login` and `/register`, and return `${url.pathname}${url.search}${url.hash}`. Update `navigate` and `replaceNavigation` to pass the complete target to History while assigning only the normalized `URL.pathname` to `router.path`.

- [ ] **Step 5: Implement app-level auth recovery**

In `App.svelte`:

- import `replaceNavigation` and `Alert`.
- while initial auth is unavailable, render a centered recovery card with the generic session-restore message and a Retry button calling `auth.retryRefresh()`; keep `auth.initializing` false so the spinner does not become permanent.
- when authenticated and `renewalUnavailable`, render one persistent `Alert` below `AppHeader`, preserving current `Page`, accounts, and notifications.
- after retry succeeds, call existing `accounts.load()` and `notifications.reconcile("manual")` rather than creating a new refresh service.
- use `replaceNavigation` for guards.
- before replacing a protected path, store `safeReturnPath(router.path + window.location.search)` as `returnTo`; add `sessionExpired` only when `auth.consumeSessionExpired()` returns true.

Do not reset session stores while `renewalUnavailable` is true.

- [ ] **Step 6: Implement one-shot login state consumption**

Extend the existing state-consumption effect in `LoginPage.svelte`: copy validated `returnTo`, display `"Your session expired. Sign in again to continue."` for `sessionExpired`, remove consumed keys with `replaceNavigationState`, and after login call `navigate(returnTo ?? "/")`.

- [ ] **Step 7: Validate changed Svelte files**

Run the Svelte autofixer for `frontend/src/App.svelte` and `frontend/src/lib/pages/LoginPage.svelte`; apply every correctness/accessibility finding and rerun until no issues remain.

- [ ] **Step 8: Run focused recovery tests**

Run: `mise run frontend:test -- src/lib/router.svelte.test.ts src/App.test.ts src/lib/pages/LoginPage.test.ts`

Expected: PASS.

Run: `mise run frontend:check`

Expected: PASS.

- [ ] **Step 9: Commit session recovery UI**

```bash
git add frontend/src/lib/router.svelte.ts frontend/src/lib/router.svelte.test.ts frontend/src/App.svelte frontend/src/App.test.ts frontend/src/lib/pages/LoginPage.svelte frontend/src/lib/pages/LoginPage.test.ts
git commit -m "feat(frontend): add recoverable session handling"
```

---

### Task 6: Consistent Page Errors And Recovery Actions

**Files:**
- Modify: `frontend/src/lib/stores/accounts.svelte.ts:1-78`
- Modify: `frontend/src/lib/stores/accounts.svelte.test.ts`
- Modify: `frontend/src/lib/stores/notifications.svelte.ts` error assignments only
- Modify: `frontend/src/lib/stores/notifications.svelte.test.ts`
- Modify: `frontend/src/lib/pages/DashboardPage.svelte:99-117`
- Modify: `frontend/src/lib/pages/NewAccountPage.svelte:64-113,129-159`
- Modify: `frontend/src/lib/pages/TransferPage.svelte` catch/load error branches only
- Modify: `frontend/src/lib/pages/AccountHistoryPage.svelte:37-77,122-135`
- Modify: `frontend/src/lib/pages/VerifyEmailPage.svelte:10-39,72-87`
- Create: `frontend/src/lib/pages/VerifyEmailPage.test.ts`
- Modify: `frontend/src/lib/pages/NotificationsPage.svelte:1-69,112-123`
- Modify: `frontend/src/lib/components/NotificationBell.svelte:1-44,113-125`
- Modify: corresponding existing `*.test.ts` files for every changed component/page/store
- Test: focused frontend unit/component tests

**Interfaces:**
- Consumes: `toMessage(unknown): string` from Task 3.
- Preserves: store rollback, stale-generation checks, retained visible data, notification reconciliation, and transfer idempotency key behavior.

- [ ] **Step 1: Write failing safe-copy and retry tests**

Add representative tests rather than snapshots:

- Dashboard account load failure: `"We couldn't load your accounts. SimpleBank is temporarily unavailable. Please try again."` plus Retry.
- Account history refresh failure retains the current account/activity and shows `"We couldn't refresh this account's activity."` plus Retry.
- New account policy failure and account inventory failure use one context prefix each, without duplicated punctuation.
- Transfer `insufficient_balance` and `daily_limit_exceeded` show code-specific actionable text while preserving the unchanged retry idempotency key.
- Verification network failure retains the captured request data in component state and presents Retry; `invalid_verification_link` presents no futile network retry and directs the user to sign in.
- Notification popover mutation error is announced by `role="alert"`, shows safe text, and retries the saved operation.
- Notifications page does not deduplicate errors by comparing message strings; mutation and reconciliation ownership yields at most one visible alert per failed action.

- [ ] **Step 2: Run focused tests and verify current inconsistent behavior fails**

Run:

```bash
mise run frontend:test -- \
  src/lib/stores/accounts.svelte.test.ts \
  src/lib/stores/notifications.svelte.test.ts \
  src/lib/pages/DashboardPage.test.ts \
  src/lib/pages/NewAccountPage.test.ts \
  src/lib/pages/TransferPage.test.ts \
  src/lib/pages/AccountHistoryPage.test.ts \
  src/lib/pages/NotificationsPage.test.ts \
  src/lib/components/NotificationBell.test.ts
```

Expected: FAIL on old raw strings, missing verification retry coverage, and notification alert semantics.

- [ ] **Step 3: Normalize store-level generic messages**

Keep stores responsible for operation state and generic classified messages only. Continue using `toMessage` in account load and notification reconciliation/load-more/mutations. Remove any direct `Error.message` access. Do not add page-specific nouns to stores.

For notification mutations, choose one owner: retain optimistic rollback/reconciliation and the thrown classified error in the store, but do not also assign the same failure to the store's page-load `error`. This removes string-equality deduplication in `NotificationsPage`.

- [ ] **Step 4: Add page context and recovery without changing successful flows**

Use the shared `Alert` everywhere. Prefix load operations once; leave code-specific submission messages unprefixed where the action is already obvious. Keep form values after submission errors. Keep account/activity data visible during refresh failures.

Refactor email verification into a reusable `verify(id, code)` function so Retry can repeat the same sanitized query after the browser URL has already been cleared. Use `isRetryable` from Task 3 to show Retry only for classified transient failures; invalid/incomplete links keep the sign-in action.

- [ ] **Step 5: Validate all changed Svelte files**

Run the Svelte autofixer individually for each changed `.svelte` file. Resolve all correctness and accessibility findings, then rerun each until clean.

- [ ] **Step 6: Run focused page/store tests**

Run the command from Step 2 plus the new verification test file if created:

```bash
mise run frontend:test -- src/lib/pages/VerifyEmailPage.test.ts
```

Expected: PASS.

Run: `mise run frontend:check`

Expected: PASS.

- [ ] **Step 7: Commit consistent user-facing recovery**

```bash
git add frontend/src/lib/stores/accounts.svelte.ts frontend/src/lib/stores/accounts.svelte.test.ts frontend/src/lib/stores/notifications.svelte.ts frontend/src/lib/stores/notifications.svelte.test.ts frontend/src/lib/pages/DashboardPage.svelte frontend/src/lib/pages/DashboardPage.test.ts frontend/src/lib/pages/NewAccountPage.svelte frontend/src/lib/pages/NewAccountPage.test.ts frontend/src/lib/pages/TransferPage.svelte frontend/src/lib/pages/TransferPage.test.ts frontend/src/lib/pages/AccountHistoryPage.svelte frontend/src/lib/pages/AccountHistoryPage.test.ts frontend/src/lib/pages/VerifyEmailPage.svelte frontend/src/lib/pages/VerifyEmailPage.test.ts frontend/src/lib/pages/NotificationsPage.svelte frontend/src/lib/pages/NotificationsPage.test.ts frontend/src/lib/components/NotificationBell.svelte frontend/src/lib/components/NotificationBell.test.ts
git commit -m "feat(frontend): make errors actionable and consistent"
```

---

### Task 7: Sanitized Root Error Boundary

**Files:**
- Create: `frontend/src/Root.svelte`
- Create: `frontend/src/Root.test.ts`
- Create: `frontend/src/lib/components/AppErrorFallback.svelte`
- Create: `frontend/src/lib/components/AppErrorFallback.test.ts`
- Modify: `frontend/src/main.ts:1-13`
- Test: `frontend/src/Root.test.ts`
- Test: `frontend/src/lib/components/AppErrorFallback.test.ts`

**Interfaces:**
- Produces: `AppErrorFallback` props `{ reset: () => void; reload?: () => void }`.
- Produces: a root `<svelte:boundary>` whose failed snippet never renders the thrown value.

- [ ] **Step 1: Write the fallback component test first**

Render `AppErrorFallback` with spies and assert:

```typescript
expect(screen.getByRole("alert")).toHaveTextContent("We couldn't display SimpleBank.");
expect(screen.getByRole("alert")).not.toHaveTextContent("database password leaked");
await user.click(screen.getByRole("button", { name: "Try again" }));
expect(reset).toHaveBeenCalledOnce();
await user.click(screen.getByRole("button", { name: "Reload page" }));
expect(reload).toHaveBeenCalledOnce();
```

- [ ] **Step 2: Write a root-boundary test with a throwing child seam**

Give `Root.svelte` an optional test-only component prop defaulting to `App`:

```svelte
let { content = App }: { content?: Component } = $props();
```

Create this test fixture under `frontend/src/test/ThrowingComponent.svelte`:

```svelte
<script module lang="ts">
  let shouldThrow = true;
  export function allowRender() {
    shouldThrow = false;
  }
</script>

<script lang="ts">
  if (shouldThrow) throw new Error("database password leaked");
</script>

<p>Recovered content</p>
```

Render `Root` with `content={ThrowingComponent}` and assert the sanitized fallback appears. Call `allowRender()`, click Try again, and assert `Recovered content` mounts.

- [ ] **Step 3: Run boundary tests and verify they fail**

Run: `mise run frontend:test -- src/Root.test.ts src/lib/components/AppErrorFallback.test.ts`

Expected: FAIL because neither component exists.

- [ ] **Step 4: Implement the fallback and root boundary**

`AppErrorFallback.svelte` uses the existing shell styling, one `role="alert"`, the exact generic heading, a short recovery sentence, and two buttons. Default `reload` calls `window.location.reload()`.

`Root.svelte` uses:

```svelte
<svelte:boundary onerror={reportError}>
  <Content />
  {#snippet failed(_error, reset)}
    <AppErrorFallback {reset} />
  {/snippet}
</svelte:boundary>
```

`reportError` may call `console.error` only when `import.meta.env.DEV`; it must never assign the error text to reactive state or markup. Change `main.ts` to mount `Root` instead of `App`.

- [ ] **Step 5: Validate new Svelte files**

Run the Svelte autofixer for `Root.svelte`, `AppErrorFallback.svelte`, and the throwing fixture; resolve and rerun until clean.

- [ ] **Step 6: Run boundary and app tests**

Run: `mise run frontend:test -- src/Root.test.ts src/lib/components/AppErrorFallback.test.ts src/App.test.ts`

Expected: PASS.

Run: `mise run frontend:check`

Expected: PASS.

- [ ] **Step 7: Commit the crash fallback**

```bash
git add frontend/src/Root.svelte frontend/src/Root.test.ts frontend/src/test/ThrowingComponent.svelte frontend/src/lib/components/AppErrorFallback.svelte frontend/src/lib/components/AppErrorFallback.test.ts frontend/src/main.ts
git commit -m "feat(frontend): add sanitized crash recovery"
```

---

### Task 8: Browser Coverage And Completion Gates

**Files:**
- Modify: `frontend/e2e/support/mock-api.ts`
- Modify: `frontend/e2e/rate-limit.spec.ts`
- Modify: `frontend/e2e/accessibility.spec.ts`
- Modify: `frontend/e2e/notifications.spec.ts` only if its mocked failure shape is affected
- Test: complete backend and frontend suites

**Interfaces:**
- Consumes: final API error response and UI behavior from Tasks 1-7.
- Produces: browser proof for rate limiting, accessible retry, session recovery, and redaction.

- [ ] **Step 1: Update mock failures to the additive API contract**

Change mocked JSON failures from `{ error: "..." }` to stable fixtures such as:

```typescript
{ code: "rate_limited", error: "rate limit exceeded" }
{ code: "internal_error", error: "internal server error" }
```

Do not use custom backend text as an expected UI string.

- [ ] **Step 2: Add browser assertions for accessible recovery**

Extend the existing accessibility recovery flow to assert that a `503/internal_error` produces the calm generic server message, exposes a named Retry button, passes `axe`, and succeeds after the mock changes to `200`. Extend the rate-limit test to keep its countdown assertion with `code: "rate_limited"`.

Add one session-restore browser case: intercept `/tokens/renew` with `503`, load `/transfer`, assert startup recovery appears instead of login, then switch the intercept to `200`, click Retry, and assert the Send money heading appears.

- [ ] **Step 3: Run focused browser tests**

Run: `mise run frontend:test:e2e -- rate-limit.spec.ts accessibility.spec.ts`

Expected: PASS.

- [ ] **Step 4: Run every changed Svelte file through the autofixer once more**

Use the Svelte autofixer on the complete changed `.svelte` file list from Tasks 5-7. Expected: no remaining issues or suggestions.

- [ ] **Step 5: Run frontend completion gates**

Run each command independently:

```bash
mise run frontend:check
mise run frontend:lint
mise run frontend:format:check
mise run frontend:test
mise run frontend:test:e2e
```

Expected: all PASS.

- [ ] **Step 6: Run backend completion gates**

Run each command independently:

```bash
mise run golangci-lint:fmt
mise run golangci-lint
mise run test:unit
```

Expected: all PASS. Integration tests are not required because no SQL, migration, locking, or transaction behavior changes.

- [ ] **Step 7: Inspect final diff and commit browser coverage**

Verify no raw server/native error text is asserted as user copy and no unrelated files are staged.

```bash
git add frontend/e2e
git commit -m "test: cover user-facing error recovery"
```

- [ ] **Step 8: Report proof and residual risks**

Report the stable API contract, transient-session behavior, representative user copy, root fallback, and exact gate results. Note only genuine omissions or failures; do not claim integration testing was run.
