# Svelte Frontend Correctness Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the Svelte SPA against cross-session retries, stale account loads, changed transfer retries, unavailable prerequisite data, lost logout feedback, and known-invalid auth submissions.

**Architecture:** Keep the current Svelte 5 SPA modules and strengthen invariants in their existing owners. `AuthStore` defines auth epochs, the API client binds refresh/retry to an epoch, `AccountsStore` orders cache loads, and pages own validated user intent and prerequisite-state presentation.

**Tech Stack:** Svelte 5.56 runes, TypeScript 6, Vite 8, Vitest 4, Testing Library Svelte, Playwright, axe, Tailwind CSS 4, daisyUI 5.

**Spec:** `docs/superpowers/specs/2026-08-22-svelte-frontend-correctness-hardening-design.md`

## Global Constraints

- Preserve the current SPA architecture; do not add a request coordinator, dependency, interface, or service layer.
- Preserve session-scoped account reset generation checks.
- Money remains integer minor units; transfer intent compares the parsed integer amount and currency.
- The backend remains authoritative for authentication and transfer validation.
- Use no experimental async Svelte and perform no visual redesign or optional component cleanup.
- Before editing any `.svelte` file, use the `svelte-file-editor` subagent with the official Svelte MCP and load the Svelte core best-practices skill.
- Before changing daisyUI markup, load the daisyUI, usage, and color skills. For Tasks 4-6, read the alert, loading, button, fieldset, and input component guides before editing.
- After each `.svelte` edit, run the official Svelte autofixer until it reports no actionable issue.
- Use `mise` tasks from the repository root. Do not run direct package-manager substitutes.
- Do not modify generated backend code or backend APIs.
- Do not create commits unless the user explicitly requests them during execution.

## File Structure

- `frontend/src/lib/stores/auth.svelte.ts`: auth generation transitions and immediate local logout invalidation.
- `frontend/src/lib/api/client.ts`: generation-scoped token refresh and retry.
- `frontend/src/lib/stores/accounts.svelte.ts`: latest-load-wins account cache state.
- `frontend/src/lib/pages/TransferPage.svelte`: transfer-intent idempotency and account prerequisite UI.
- `frontend/src/lib/pages/NewAccountPage.svelte`: account inventory and opening-policy prerequisite UI.
- `frontend/src/lib/router.svelte.ts`: reactive history state and replacement for same-path one-shot notices.
- `frontend/src/lib/components/AppHeader.svelte`: sign-out action only; no transient failure ownership.
- `frontend/src/lib/pages/LoginPage.svelte`: logout notice consumption and login field errors.
- `frontend/src/lib/pages/RegisterPage.svelte`: registration field errors.
- `frontend/src/lib/auth-validation.ts`: pure normalization and validation shared by auth forms.
- Existing colocated tests remain responsible for each module; add `LoginPage.test.ts`, `RegisterPage.test.ts`, `auth-validation.test.ts`.
- `frontend/e2e/accessibility.spec.ts`: representative mobile axe checks for new validation and load-error states.

---

### Task 1: Bind Authenticated Retries To Their Session

**Files:**
- Modify: `frontend/src/lib/stores/auth.svelte.ts:15-103`
- Modify: `frontend/src/lib/api/client.ts:7-70`
- Test: `frontend/src/lib/stores/auth.svelte.test.ts`
- Test: `frontend/src/lib/api/client.test.ts`
- Reference: `docs/superpowers/specs/2026-08-22-svelte-frontend-correctness-hardening-design.md`
- Reference: `docs/superpowers/plans/2026-08-22-svelte-frontend-correctness-hardening.md`

**Interfaces:**
- Produces: `AuthStore.generation: number` as a read-only getter.
- Produces: `AuthStore.clear(): void` as an auth-invalidating transition that advances generation.
- Consumes: existing `AuthStore.tryRefresh(): Promise<boolean>` and `request<T>()` signatures unchanged.

- [ ] **Step 1: Add failing auth-generation transition tests**

Add tests proving `generation` advances on `clear`, login start, failed/absent refresh invalidation, and logout. Stub `navigate` or reset history after logout tests so routing does not leak between cases. Update the existing pending-logout assertion to require immediate clearing:

```ts
it("invalidates local auth before server logout settles", async () => {
  let resolveLogout!: (value: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveLogout = resolve;
        }),
    ),
  );
  const store = new AuthStore();
  store.user = verifiedUser;
  store.accessToken = "access";
  const generation = store.generation;

  const logoutTask = store.logout();

  expect(store.user).toBeNull();
  expect(store.accessToken).toBeNull();
  expect(store.generation).toBe(generation + 1);
  resolveLogout(jsonResponse(204, undefined));
  await logoutTask;
});
```

- [ ] **Step 2: Run the auth-store test and verify failure**

Run: `mise run frontend:test -- src/lib/stores/auth.svelte.test.ts`

Expected: FAIL because `generation` is not public and logout still clears only after the request settles.

- [ ] **Step 3: Implement one generation advance per auth transition**

Refactor `AuthStore` around a non-advancing private state clear:

```ts
get generation(): number {
  return this.#generation;
}

#clearState(): void {
  this.user = null;
  this.accessToken = null;
}

clear(): void {
  this.#generation += 1;
  this.#clearState();
}
```

Keep login's existing pre-request increment. In `tryRefresh`, when the captured generation is still current and refresh is absent or fails, call `clear()` rather than assigning fields. In `logout`, advance once, clear state immediately with `#clearState()`, await the unauthenticated logout request, and navigate in `finally`. Do not call public `clear()` from the same logout transition.

- [ ] **Step 4: Add failing cross-session API retry tests**

Add deferred-response tests to `client.test.ts` for both invariants:

```ts
it("does not retry a 401 after the auth generation changes", async () => {
  let resolveFirst!: (response: Response) => void;
  const fetchMock = vi.fn(
    () => new Promise<Response>((resolve) => (resolveFirst = resolve)),
  );
  vi.stubGlobal("fetch", fetchMock);
  auth.accessToken = "old-token";

  const pending = request("/transfers", { method: "POST", authenticated: true, body: {} });
  auth.clear();
  auth.accessToken = "new-token";
  resolveFirst(jsonResponse(401, { error: "expired" }));

  await expect(pending).rejects.toMatchObject({ status: 401 });
  expect(fetchMock).toHaveBeenCalledOnce();
});
```

For refresh coalescing, hold the old generation's `tryRefresh`, call `auth.clear()`, start a new authenticated request that receives 401, and assert `tryRefresh` is called twice rather than the new request joining the old promise. Resolve both deferred refreshes before awaiting either request. Retain the existing same-generation coalescing test.

- [ ] **Step 5: Run the API client test and verify failure**

Run: `mise run frontend:test -- src/lib/api/client.test.ts`

Expected: FAIL because all generations share one `refreshPromise` and retry does not recheck session identity.

- [ ] **Step 6: Scope refresh and retry to the captured generation**

Replace the global promise with an attempt record:

```ts
interface RefreshAttempt {
  generation: number;
  promise: Promise<boolean>;
}

let refreshAttempt: RefreshAttempt | null = null;

function refreshAccessToken(generation: number): Promise<boolean> {
  if (refreshAttempt?.generation === generation) return refreshAttempt.promise;

  const attempt: RefreshAttempt = {
    generation,
    promise: Promise.resolve(false),
  };
  attempt.promise = auth.tryRefresh().finally(() => {
    if (refreshAttempt === attempt) refreshAttempt = null;
  });
  refreshAttempt = attempt;
  return attempt.promise;
}
```

Capture `const generation = auth.generation` before the first authenticated send. On 401, decode the original response unless the generation is still current both before refresh and after refresh. Retry only when refresh succeeded and the generation remains equal.

- [ ] **Step 7: Run focused tests**

Run: `mise run frontend:test -- src/lib/stores/auth.svelte.test.ts src/lib/api/client.test.ts`

Expected: PASS, including same-generation coalescing and late-refresh isolation.

- [ ] **Step 8: Inspect the session-bound request checkpoint**

```bash
git diff --check -- frontend/src/lib/stores/auth.svelte.ts frontend/src/lib/stores/auth.svelte.test.ts frontend/src/lib/api/client.ts frontend/src/lib/api/client.test.ts
```

Expected: no whitespace errors. Review the focused diff before continuing; do not commit without an explicit user request.

### Task 2: Make Account Loads Latest-Wins

**Files:**
- Modify: `frontend/src/lib/stores/accounts.svelte.ts:10-43`
- Test: `frontend/src/lib/stores/accounts.svelte.test.ts`

**Interfaces:**
- Preserves: `accounts.load(): Promise<void>`.
- Preserves: `#generation` as cross-session reset protection.
- Produces: private latest-load sequence ordering for all load-published state.

- [ ] **Step 1: Add failing out-of-order load tests**

Create two deferred fetches, start two loads, and resolve the second first with `freshAccount`. Assert the first remains unable to publish items or clear loading:

```ts
const first = accounts.load();
const second = accounts.load();
resolveSecond(jsonResponse(200, [freshAccount]));
await second;
expect(accounts.items).toEqual([freshAccount]);

resolveFirst(jsonResponse(200, [staleAccount]));
await first;
expect(accounts.items).toEqual([freshAccount]);
expect(accounts.loading).toBe(false);
```

Add a second assertion that resolving the older request while the newer request is pending leaves `accounts.loading === true`.

- [ ] **Step 2: Run the focused test and verify failure**

Run: `mise run frontend:test -- src/lib/stores/accounts.svelte.test.ts`

Expected: FAIL because both loads currently share only the session generation.

- [ ] **Step 3: Implement a separate load sequence**

Add `#loadSequence = 0`. At load start capture `const sequence = ++this.#loadSequence` and publish any success, error, or final loading update only when:

```ts
this.#generation === generation && this.#loadSequence === sequence
```

Increment `#loadSequence` in `reset()` as well, so pending work is immediately superseded even independently of the session-generation check.

- [ ] **Step 4: Run focused store tests**

Run: `mise run frontend:test -- src/lib/stores/accounts.svelte.test.ts`

Expected: PASS for reset isolation, create isolation, response ordering, and loading ownership.

- [ ] **Step 5: Inspect the latest-load checkpoint**

```bash
git diff --check -- frontend/src/lib/stores/accounts.svelte.ts frontend/src/lib/stores/accounts.svelte.test.ts
```

Expected: no whitespace errors and only account-load ordering changes in the focused diff.

### Task 3: Bind Transfer Keys To Normalized Intent

**Files:**
- Modify: `frontend/src/lib/pages/TransferPage.svelte:13-110`
- Test: `frontend/src/lib/pages/TransferPage.test.ts`

**Interfaces:**
- Produces locally: `TransferIntent` with `from_account_id`, `to_account_id`, `amount`, and `currency`.
- Preserves: API payload and backend idempotency contract.

- [ ] **Step 1: Add failing changed-intent retry tests**

Parameterize edits for recipient and amount, and add a two-account case for source/currency. Each case submits once to a 503, edits one immutable field, submits again, and compares request bodies:

```ts
const firstBody = jsonRequestBody(fetchMock.mock.calls[1]);
const changedBody = jsonRequestBody(fetchMock.mock.calls[2]);
expect(changedBody.idempotency_key).not.toBe(firstBody.idempotency_key);
```

Keep the existing unchanged-retry test asserting complete body equality. Add one invalid-submission case asserting `crypto.randomUUID` has not been called again before the first API attempt.

- [ ] **Step 2: Run the transfer test and verify failure**

Run: `mise run frontend:test -- src/lib/pages/TransferPage.test.ts`

Expected: FAIL because the page retains one key regardless of edited transfer details.

- [ ] **Step 3: Implement intent comparison before submission**

Define the local intent and comparison:

```ts
interface TransferIntent {
  from_account_id: string;
  to_account_id: string;
  amount: number;
  currency: Currency;
}

let idempotencyKey = crypto.randomUUID();
let keyedIntent: TransferIntent | null = null;

function sameIntent(left: TransferIntent, right: TransferIntent): boolean {
  return left.from_account_id === right.from_account_id &&
    left.to_account_id === right.to_account_id &&
    left.amount === right.amount &&
    left.currency === right.currency;
}
```

After all local validation succeeds, construct the normalized intent. If `keyedIntent` exists and differs, rotate the key. Then bind `keyedIntent = intent` before calling `request`. Send `{ ...intent, idempotency_key: idempotencyKey }`. On success rotate the key and set `keyedIntent = null`; on API failure retain both.

- [ ] **Step 4: Validate the Svelte component**

Run the official Svelte autofixer on `TransferPage.svelte`. Apply only actionable correctness suggestions; retain intentional network and mount side effects.

- [ ] **Step 5: Run the focused transfer test**

Run: `mise run frontend:test -- src/lib/pages/TransferPage.test.ts`

Expected: PASS for unchanged retries, each changed intent field, invalid submissions, and success receipts.

- [ ] **Step 6: Inspect the idempotency checkpoint**

```bash
git diff --check -- frontend/src/lib/pages/TransferPage.svelte frontend/src/lib/pages/TransferPage.test.ts
```

Expected: no whitespace errors and no transfer-policy or visual changes.

### Task 4: Gate Account-Dependent Forms On Inventory

**Files:**
- Modify: `frontend/src/lib/pages/TransferPage.svelte:27-41,154-212`
- Modify: `frontend/src/lib/pages/NewAccountPage.svelte:30-60,125-194`
- Test: `frontend/src/lib/pages/TransferPage.test.ts`
- Test: `frontend/src/lib/pages/NewAccountPage.test.ts`

**Interfaces:**
- Consumes: `accounts.loaded`, `accounts.loading`, `accounts.error`, and `accounts.load()`.
- Produces locally: page-specific `loadAccounts(): Promise<void>` callbacks that initialize dependent selections after successful loads.

- [ ] **Step 1: Add failing transfer inventory-state tests**

Set `accounts.loaded = false` and mock `accounts.load()` with a pending promise. Assert the form's `Send transfer` button and account combobox are absent while a status contains `Loading your accounts`. Then set `accounts.error = "offline"`, resolve the load, and assert an alert contains the error and a `Retry` button calls `accounts.load()` again.

- [ ] **Step 2: Add failing account-creation inventory-state tests**

With opening policy successful but `accounts.loaded = false`, assert currency radios and `Create account` are absent while inventory loads. For a failed load, set `accounts.error`, resolve the mock, assert retry UI, and verify no currency is offered as if the inventory were empty.

- [ ] **Step 3: Run both page tests and verify failure**

Run: `mise run frontend:test -- src/lib/pages/TransferPage.test.ts src/lib/pages/NewAccountPage.test.ts`

Expected: FAIL because both pages currently render forms from an unknown or failed inventory.

- [ ] **Step 4: Gate the transfer form and initialize selection after retry**

Extract a `loadAccounts` function that awaits `accounts.load()` when needed and, only when `accounts.loaded`, selects `accounts.transferFromId ?? accounts.items[0]?.id ?? ""` and clears `transferFromId`. Render these branches before the existing empty/form branches:

```svelte
{#if !accounts.loaded}
  {#if accounts.error}
    <Alert variant="error">
      Couldn't load your accounts. {accounts.error}
      <button type="button" class="btn btn-ghost min-h-11" onclick={loadAccounts}>Retry</button>
    </Alert>
  {:else}
    <Alert variant="info">
      <span class="loading loading-spinner loading-sm" aria-hidden="true"></span>
      Loading your accounts…
    </Alert>
  {/if}
{:else if accounts.items.length === 0}
  <!-- existing empty state -->
{:else}
  <!-- existing transfer form -->
{/if}
```

Start transfer-limit loading independently so its non-fatal policy remains parallel.

- [ ] **Step 5: Gate account creation on both prerequisites**

Keep opening-policy loading parallel with account loading. Add `accountsReady = $derived(accounts.loaded && accounts.error === null)` and include it in `formDisabled`. Render account inventory loading/error UI before the currency choices. The inventory retry calls an account loader that also corrects `currency` to the first truly available value after success.

- [ ] **Step 6: Validate both Svelte components**

Run the official Svelte autofixer on `TransferPage.svelte` and `NewAccountPage.svelte` until neither reports actionable issues.

- [ ] **Step 7: Run focused page tests**

Run: `mise run frontend:test -- src/lib/pages/TransferPage.test.ts src/lib/pages/NewAccountPage.test.ts`

Expected: PASS for loading, failure, retry, empty, ready, policy, and transfer-idempotency states.

- [ ] **Step 8: Inspect the prerequisite-state checkpoint**

```bash
git diff --check -- frontend/src/lib/pages/TransferPage.svelte frontend/src/lib/pages/TransferPage.test.ts frontend/src/lib/pages/NewAccountPage.svelte frontend/src/lib/pages/NewAccountPage.test.ts
```

Expected: no whitespace errors and only account prerequisite behavior plus tests.

### Task 5: Preserve Logout Failure Feedback After Navigation

**Files:**
- Modify: `frontend/src/lib/router.svelte.ts:15-29`
- Modify: `frontend/src/lib/stores/auth.svelte.ts:89-98`
- Modify: `frontend/src/lib/components/AppHeader.svelte:19-62,126-130`
- Modify: `frontend/src/lib/pages/LoginPage.svelte:16-48`
- Test: `frontend/src/lib/router.svelte.test.ts`
- Test: `frontend/src/lib/components/AppHeader.test.ts`
- Test: `frontend/src/App.test.ts`

**Interfaces:**
- Produces: `router.state: Record<string, unknown>` synchronized by `navigate` and `popstate`.
- Produces: `replaceNavigationState(state: Record<string, unknown>): void` to update the current history entry and reactive snapshot together.
- Produces: logout navigation state `{ logoutFailed: true }` only when the server request fails.
- Preserves: local logout always succeeds from the user's perspective and credentials are never restored.

- [ ] **Step 1: Add failing reactive history-state tests**

Extend `router.svelte.test.ts` to assert `router.state` changes on `navigate` and after a `popstate` whose history entry has state. This ensures same-path navigation can update mounted pages.

- [ ] **Step 2: Add a failing integrated logout-failure test**

In `App.test.ts`, expand `accountsMock` with the state and methods read by the dashboard (`items`, `loaded`, `loading`, `error`, `load`, and `reset`). Seed authenticated state and `/`, reject `/users/logout`, render `App`, and click `Sign out`. Assert the login heading appears, local auth is null, and an alert says local sign-out succeeded but the server request could not be completed. Assert `history.state.logoutFailed` is absent after the notice is consumed.

- [ ] **Step 3: Run router and app tests and verify failure**

Run: `mise run frontend:test -- src/lib/router.svelte.test.ts src/App.test.ts src/lib/components/AppHeader.test.ts`

Expected: FAIL because router state is not reactive and the header owns an error that unmounts.

- [ ] **Step 4: Make history state reactive**

Extend router state without changing `navigate`'s signature:

```ts
function historyState(): Record<string, unknown> {
  const state: unknown = window.history.state;
  return state !== null && typeof state === "object" ? { ...state } : {};
}

export const router = $state({
  path: normalize(window.location.pathname),
  state: historyState(),
});
```

Assign `router.state = state` in `navigate`. On `popstate`, refresh both `router.path` and `router.state`.

Add state replacement for the mounted-login case:

```ts
export function replaceNavigationState(state: Record<string, unknown>): void {
  window.history.replaceState(state, "", window.location.href);
  router.state = state;
}
```

- [ ] **Step 5: Navigate with a one-shot logout result**

In `AuthStore.logout`, retain immediate local invalidation, capture a boolean failure, and navigate in `finally`:

```ts
let logoutFailed = false;
try {
  await request<void>("/users/logout", { method: "POST" });
} catch {
  logoutFailed = true;
} finally {
  navigate("/login", logoutFailed ? { logoutFailed: true } : {});
}
```

Resolve logout normally rather than rethrowing the network error; local sign-out is complete.

- [ ] **Step 6: Move feedback from the header to login**

Remove `logoutError` and its alert from `AppHeader`; reset accounts in `finally`. In `LoginPage`, use an effect to observe `router.state.logoutFailed === true`, retain a local `logoutFailed` notice, and remove only that key through `replaceNavigationState`. The effect is required because immediate local auth invalidation can mount login before logout navigation publishes its state. Do not persist the notice or put it in auth state:

```ts
let logoutFailed = $state(false);

$effect(() => {
  if (router.state.logoutFailed !== true) return;
  logoutFailed = true;
  const remainingState = { ...router.state };
  delete remainingState.logoutFailed;
  replaceNavigationState(remainingState);
});
```

Render:

```svelte
{#if logoutFailed}
  <Alert variant="error">
    You were signed out locally, but SimpleBank couldn't complete the server sign-out request.
  </Alert>
{/if}
```

Both the router snapshot and browser history entry are cleared immediately so the effect cannot repeat and reload/back does not restore the notice.

- [ ] **Step 7: Update header unit expectations**

Replace the isolated failed-logout alert test with an assertion that the header resets account state and does not retain credentials. Keep user-visible failure presentation covered at `App` integration level.

- [ ] **Step 8: Validate changed Svelte components**

Run the official Svelte autofixer on `AppHeader.svelte` and `LoginPage.svelte` until no actionable issue remains.

- [ ] **Step 9: Run focused navigation/logout tests**

Run: `mise run frontend:test -- src/lib/router.svelte.test.ts src/lib/stores/auth.svelte.test.ts src/lib/components/AppHeader.test.ts src/App.test.ts`

Expected: PASS, including immediate local invalidation and one-shot integrated feedback.

- [ ] **Step 10: Inspect the logout-feedback checkpoint**

```bash
git diff --check -- frontend/src/lib/router.svelte.ts frontend/src/lib/router.svelte.test.ts frontend/src/lib/stores/auth.svelte.ts frontend/src/lib/stores/auth.svelte.test.ts frontend/src/lib/components/AppHeader.svelte frontend/src/lib/components/AppHeader.test.ts frontend/src/lib/pages/LoginPage.svelte frontend/src/App.test.ts
```

Expected: no whitespace errors and no persisted or auth-owned notice state.

### Task 6: Add Accessible Auth Form Validation

**Files:**
- Create: `frontend/src/lib/auth-validation.ts`
- Create: `frontend/src/lib/auth-validation.test.ts`
- Create: `frontend/src/lib/pages/LoginPage.test.ts`
- Create: `frontend/src/lib/pages/RegisterPage.test.ts`
- Modify: `frontend/src/lib/pages/LoginPage.svelte:11-35,40-59`
- Modify: `frontend/src/lib/pages/RegisterPage.svelte:11-37,41-64`

**Interfaces:**
- Produces: `validateLogin(input: LoginInput): ValidationResult<LoginInput>`.
- Produces: `validateRegistration(input: RegistrationInput): ValidationResult<RegistrationInput>`.
- `ValidationResult<T>` contains normalized `values: T` and field-keyed `errors: Partial<Record<keyof T, string>>`.

- [ ] **Step 1: Write failing pure validation tests**

Cover whitespace-only required fields, non-alphanumeric usernames, invalid email, 14-character password, and a password over 72 UTF-8 bytes. Also prove values are normalized without trimming passwords:

```ts
expect(
  validateRegistration({
    fullName: " Alice Smith ",
    username: " alice01 ",
    email: " alice@example.com ",
    password: "correct horse battery staple",
  }),
).toEqual({
  values: {
    fullName: "Alice Smith",
    username: "alice01",
    email: "alice@example.com",
    password: "correct horse battery staple",
  },
  errors: {},
});
```

- [ ] **Step 2: Run the validation test and verify failure**

Run: `mise run frontend:test -- src/lib/auth-validation.test.ts`

Expected: FAIL because the module does not exist.

- [ ] **Step 3: Implement the pure validation module**

Use ASCII alphanumeric username matching, a small email-shape check, and `TextEncoder` for the backend's byte cap:

```ts
export interface LoginInput { username: string; password: string }
export interface RegistrationInput extends LoginInput { fullName: string; email: string }
export interface ValidationResult<T> {
  values: T;
  errors: Partial<Record<keyof T, string>>;
}

const ALPHANUMERIC = /^[A-Za-z0-9]+$/;
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const utf8 = new TextEncoder();
```

Return exact actionable messages: `Enter your username.`, `Use letters and numbers only.`, `Enter your password.`, `Enter your full name.`, `Enter a valid email address.`, `Use at least 15 characters.`, and `Use no more than 72 bytes.`

- [ ] **Step 4: Run pure validation tests**

Run: `mise run frontend:test -- src/lib/auth-validation.test.ts`

Expected: PASS.

- [ ] **Step 5: Write failing login and registration component tests**

For each page, render it, submit known-invalid values, assert `fetch` was not called, and assert the first invalid `TextField` is focused with `aria-invalid="true"` and a connected alert. Then edit the field and assert its error clears. Add valid-submit tests that inspect the normalized request JSON.

- [ ] **Step 6: Run page tests and verify failure**

Run: `mise run frontend:test -- src/lib/pages/LoginPage.test.ts src/lib/pages/RegisterPage.test.ts`

Expected: FAIL because both pages submit without explicit field validation.

- [ ] **Step 7: Integrate validation into login**

Store `usernameError` and `passwordError`, clear each from its `TextField.oninput`, and call `validateLogin` before setting `submitting`. If errors exist, assign them and return without API work. Otherwise submit normalized username and the unchanged password. Pass each error through `TextField error={...}`.

- [ ] **Step 8: Integrate validation into registration**

Store one error per field, use `validateRegistration` before setting `submitting`, and submit normalized values only when `errors` is empty. Preserve the existing accepted-registration navigation state.

- [ ] **Step 9: Validate both Svelte forms**

Run the official Svelte autofixer on `LoginPage.svelte` and `RegisterPage.svelte` until no actionable issue remains.

- [ ] **Step 10: Run focused auth-form tests**

Run: `mise run frontend:test -- src/lib/auth-validation.test.ts src/lib/pages/LoginPage.test.ts src/lib/pages/RegisterPage.test.ts`

Expected: PASS for invalid no-request behavior, normalization, field associations, focus, clearing, and valid submission.

- [ ] **Step 11: Inspect the auth-validation checkpoint**

```bash
git diff --check -- frontend/src/lib/auth-validation.ts frontend/src/lib/auth-validation.test.ts frontend/src/lib/pages/LoginPage.svelte frontend/src/lib/pages/LoginPage.test.ts frontend/src/lib/pages/RegisterPage.svelte frontend/src/lib/pages/RegisterPage.test.ts
```

Expected: no whitespace errors and no duplicated backend-only policy.

### Task 7: Add Browser Accessibility Proof And Run Completion Gates

**Files:**
- Modify: `frontend/e2e/accessibility.spec.ts`
- Test: all frontend sources and tests changed above.

**Interfaces:**
- Consumes: existing `expectNoAccessibilityViolations(page)` helper and Playwright API mocks.
- Produces: browser proof for representative auth validation and account-load failure states.

- [ ] **Step 1: Add registration validation axe coverage**

At a 320x800 viewport with renew mocked as 204, open `/register`, submit empty, assert `Full name` is focused and invalid, then run `expectNoAccessibilityViolations(page)`.

- [ ] **Step 2: Add account-load failure axe coverage**

Mock authenticated renew, return 503 for `**/api/v1/accounts?*`, open `/transfer`, assert the retryable account error and `Retry` button are visible, then run axe at 320x800. The shared alert/button structure covers the browser-level failure presentation; `NewAccountPage` retains its detailed component test.

- [ ] **Step 3: Run the focused browser file and verify it passes**

Run: `mise run frontend:test:e2e -- e2e/accessibility.spec.ts`

Expected: PASS at configured Chromium projects with no axe violations or horizontal overflow regressions.

- [ ] **Step 4: Run Svelte and TypeScript checks**

Run: `mise run frontend:check`

Expected: PASS with 0 errors and 0 warnings.

- [ ] **Step 5: Run lint and formatting checks**

Run: `mise run frontend:lint`

Expected: PASS.

Run: `mise run frontend:format:check`

Expected: PASS. If formatting fails, run `mise run frontend:format`, inspect only intended changes, then rerun both check commands.

- [ ] **Step 6: Run all frontend unit tests**

Run: `mise run frontend:test`

Expected: PASS with no failed or skipped regression tests.

- [ ] **Step 7: Run the complete browser suite**

Run: `mise run frontend:test:e2e`

Expected: PASS for all desktop/mobile, rate-limit, history, responsive, and axe tests. If Chromium is not installed, run `mise run frontend:test:e2e:install` once and rerun the suite; do not treat a missing browser as a product failure.

- [ ] **Step 8: Inspect the final diff**

Run: `git status --short`

Run: `git diff --check`

Run: `git diff --stat`

Expected: only approved frontend hardening, tests, spec, and plan changes; no whitespace errors or generated artifacts.

- [ ] **Step 9: Inspect the browser-proof checkpoint**

```bash
git diff --check -- frontend/e2e/accessibility.spec.ts
```

Expected: no whitespace errors and only the two required browser cases.

- [ ] **Step 10: Record completion evidence**

Report the exact passing counts from Vitest and Playwright, the Svelte check warning/error count, and any residual risk. Do not claim completion if any gate was skipped or failed.
