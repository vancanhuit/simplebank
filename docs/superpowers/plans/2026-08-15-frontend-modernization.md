# Frontend Modernization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make SimpleBank's embedded Svelte SPA exact under concurrent session
renewal and monetary boundaries while refining its Tailwind CSS 4 visual system,
SPA behavior, responsiveness, and accessibility.

**Architecture:** Preserve the Vite SPA, History API router, rune-backed stores,
Go API, and PostgreSQL schema ownership. Add one shared safe-money invariant at
Go/API/SQL boundaries, one single-flight renewal primitive in the frontend API
client, focused router and lifecycle tests, then layer visual refinements onto
existing components and pages.

**Tech Stack:** Go 1.26.6, PostgreSQL 18, sqlc, Goose, Bun 1.3.14, Svelte 5.56.8,
Tailwind CSS 4.3.3, Vite 8.2.1, TypeScript 6.0.3, Vitest 4.1.10, Playwright
1.62.1, Axe 4.13.0, `@lucide/svelte`, IBM Plex Sans Variable.

**Spec:**
`docs/superpowers/specs/2026-08-15-frontend-modernization-design.md`

## Global Constraints

- Preserve all current routes, API field names, JSON number types, workflows,
  authorization rules, and embedded deployment behavior.
- Maximum monetary minor-unit value is exactly `9_007_199_254_740_991`.
- Keep every authored Svelte component in runes mode; do not introduce `on:`
  directives, `createEventDispatcher`, slots, class components,
  `<svelte:component>`, or deprecated component types.
- Keep Tailwind configuration CSS-first through `@theme`; do not add
  `tailwind.config.js`, `@config`, deprecated `theme()`, or dynamic utility-name
  interpolation.
- Add only `@fontsource-variable/ibm-plex-sans` and `@lucide/svelte`.
- Cards use at most 8px radius; controls remain at least 44px; supported browser
  widths are 320, 768, 1024, and 1440px.
- Preserve reduced-motion, forced-colors, keyboard, focus, and WCAG 2.1 AA
  behavior.
- Do not commit unless the user explicitly authorizes commits. Each task records
  a suggested atomic commit message for use only after that authorization.
- Follow current official APIs:
  https://svelte.dev/docs/svelte/v5-migration-guide,
  https://tailwindcss.com/docs/theme,
  https://lucide.dev/guide/svelte/getting-started, and
  https://fontsource.org/fonts/ibm-plex-sans/install.

## File Responsibility Map

- `internal/currency/currency.go`: shared minor-unit safe bound.
- `internal/config/config.go`: rejects unsafe monetary configuration.
- `internal/api/account.go`, `internal/api/transfer.go`: early transport-boundary
  rejection for unsafe request amounts.
- `internal/db/migrations/00004_javascript_safe_balances.sql`: persistent account
  balance invariant and reversible precondition.
- `internal/db/query/accounts.sql`: atomic balance update guard.
- `internal/db/transfer_tx.go`: domain error before an unsafe destination
  credit; row locks preserve concurrency correctness.
- `frontend/src/lib/api/client.ts`: single-flight access-token renewal.
- `frontend/src/lib/router.svelte.ts`, `frontend/src/App.svelte`: SPA history,
  title, announcement, and focus behavior.
- `frontend/src/app.css`, `frontend/src/main.ts`: Tailwind 4 theme and local font
  foundation.
- Existing files under `frontend/src/lib/components` and
  `frontend/src/lib/pages`: focused presentation refinements; no new generic UI
  layer.
- `frontend/e2e/accessibility.spec.ts`: responsive, navigation, accessibility,
  runtime-error, and font-loading proof.

---

### Task 1: Define and Validate the Safe Money Boundary

**Files:**
- Modify: `internal/currency/currency.go`
- Modify: `internal/currency/currency_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/api/account.go`
- Modify: `internal/api/account_test.go`
- Modify: `internal/api/transfer.go`
- Modify: `internal/api/transfer_test.go`

**Interfaces:**
- Produces: `currency.MaxSafeMinorUnits int64`
- Consumes: existing `Config.TransferLimits`, `Config.AccountOpeningLimits`,
  `createAccountRequest.Balance`, and `transferRequest.Amount`
- Preserves: existing JSON `number` request and response fields

- [ ] **Step 1: Add failing constant and parser-boundary tests**

Add this assertion to `internal/currency/currency_test.go`:

```go
func TestMaxSafeMinorUnitsMatchesJavaScript(t *testing.T) {
	t.Parallel()
	if MaxSafeMinorUnits != 9_007_199_254_740_991 {
		t.Fatalf("MaxSafeMinorUnits = %d", MaxSafeMinorUnits)
	}
}
```

Extend `TestParseTransferLimits` and `TestParseAccountOpeningLimits` with exact
boundary cases:

```go
if _, err := parseTransferLimits(
	`{"USD":{"max_per_transfer":9007199254740992}}`,
); err == nil {
	t.Error("unsafe max_per_transfer should error")
}
if _, err := parseTransferLimits(
	`{"USD":{"daily":9007199254740992}}`,
); err == nil {
	t.Error("unsafe daily limit should error")
}
if _, err := parseAccountOpeningLimits(
	`{"USD":9007199254740992}`,
); err == nil {
	t.Error("unsafe opening cap should error")
}
```

- [ ] **Step 2: Add failing API request-boundary tests**

In `internal/api/account_test.go`, post an opening balance of
`9007199254740992` with a fake store that fails the test if called. Assert `422`
and body `{"error":"opening balance exceeds the supported limit"}`.

In `internal/api/transfer_test.go`, build the request explicitly so amount is
not fixed by `postTransferWithKey`:

```go
body := `{"from_account_id":"` + fromID.String() +
	`","to_account_id":"` + toID.String() +
	`","amount":9007199254740992,"currency":"USD","idempotency_key":"` +
	uuid.NewString() + `"}`
```

Assert `422`, assert `TransferTx` was not called, and assert the response body
contains `amount exceeds the supported limit` even when no configured
per-transfer ceiling exists.

- [ ] **Step 3: Run focused tests and verify RED**

Run:

```bash
go test ./internal/currency ./internal/config ./internal/api
```

Expected: compile failure for undefined `MaxSafeMinorUnits`, followed by failing
unsafe-boundary cases once the constant exists.

- [ ] **Step 4: Add the shared constant and parser validation**

Add to `internal/currency/currency.go`:

```go
const MaxSafeMinorUnits int64 = 1<<53 - 1
```

Import `internal/currency` from `internal/config/config.go`. In
`parseTransferLimits`, reject either field above the constant:

```go
for code, limit := range limits {
	if limit.MaxPerTransfer > currency.MaxSafeMinorUnits {
		return nil, fmt.Errorf("max per-transfer limit for %s exceeds JavaScript safe integer", code)
	}
	if limit.Daily > currency.MaxSafeMinorUnits {
		return nil, fmt.Errorf("daily limit for %s exceeds JavaScript safe integer", code)
	}
}
```

Extend the existing opening-cap loop:

```go
if cap > currency.MaxSafeMinorUnits {
	return nil, fmt.Errorf("opening balance cap for %s exceeds JavaScript safe integer", currencyCode)
}
```

Name the loop key `currencyCode` so it does not shadow the imported package.

- [ ] **Step 5: Add API early rejections**

In `createAccount`, check the safe bound before the configured opening limit:

```go
if req.Balance > currency.MaxSafeMinorUnits {
	return echo.NewHTTPError(
		http.StatusUnprocessableEntity,
		"opening balance exceeds the supported limit",
	)
}
```

Import `internal/currency` in `internal/api/transfer.go` and check before
configured limits or store access:

```go
if req.Amount > currency.MaxSafeMinorUnits {
	return echo.NewHTTPError(
		http.StatusUnprocessableEntity,
		"amount exceeds the supported limit",
	)
}
```

- [ ] **Step 6: Run focused tests and verify GREEN**

Run:

```bash
go test ./internal/currency ./internal/config ./internal/api
```

Expected: PASS.

- [ ] **Step 7: Format and inspect the task diff**

Run:

```bash
gofmt -w internal/currency/currency.go internal/currency/currency_test.go \
  internal/config/config.go internal/config/config_test.go \
  internal/api/account.go internal/api/account_test.go \
  internal/api/transfer.go internal/api/transfer_test.go
git diff --check
git diff -- internal/currency internal/config internal/api
```

Suggested commit if authorized: `fix(api): bound monetary JSON values`

---

### Task 2: Enforce Safe Balances Transactionally

**Files:**
- Create: `internal/db/migrations/00004_javascript_safe_balances.sql`
- Modify: `internal/db/query/accounts.sql`
- Modify generated: `internal/db/sqlc/accounts.sql.go`
- Modify: `internal/db/errors.go`
- Modify: `internal/db/transfer_tx.go`
- Modify: `internal/db/transfer_tx_test.go`
- Modify: `internal/api/errors.go`
- Modify: `internal/api/errors_test.go`

**Interfaces:**
- Consumes: `currency.MaxSafeMinorUnits`
- Produces: `store.ErrBalanceLimitExceeded`
- Preserves: `AddAccountBalance(ctx, AddAccountBalanceParams) (Account, error)`
  generated signature
- Database constraint: `accounts_balance_javascript_safe`

- [ ] **Step 1: Add failing domain and integration tests**

Add an error-catalog case in `internal/api/errors_test.go` expecting
`ErrBalanceLimitExceeded` to map to `422` and
`destination balance exceeds the supported limit`.

Add two integration tests to `internal/db/transfer_tx_test.go`. Create destination
accounts directly with the required starting balances:

```go
func createTestAccountWithBalance(t *testing.T, owner string, balance int64) sqlcdb.Account {
	t.Helper()
	account, err := testStore.CreateAccount(t.Context(), sqlcdb.CreateAccountParams{
		Owner: owner, Balance: balance, Currency: currency.USD,
	})
	if err != nil {
		t.Fatal(err)
	}
	return account
}
```

`TestTransferTxAllowsExactSafeDestinationBalance` starts the destination at
`currency.MaxSafeMinorUnits-10`, transfers `10`, and asserts exact final balance.

`TestTransferTxRejectsUnsafeDestinationBalance` starts it at
`currency.MaxSafeMinorUnits-5`, transfers `10`, expects
`ErrBalanceLimitExceeded`, and asserts both account balances remain unchanged.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
go test ./internal/api
mise run compose:test:up
go test -tags=integration ./internal/db -run 'TestTransferTx(AllowsExactSafe|RejectsUnsafe)DestinationBalance' -v
mise run compose:test:down
```

Expected: compile failure for undefined `ErrBalanceLimitExceeded` or failing
unsafe-credit behavior. Run `mise run compose:test:down` even when the focused
integration test fails.

- [ ] **Step 3: Add reversible database constraint**

Create `internal/db/migrations/00004_javascript_safe_balances.sql`:

```sql
-- +goose Up
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM accounts WHERE balance > 9007199254740991) THEN
        RAISE EXCEPTION 'cannot apply JavaScript-safe balance constraint: unsafe balances exist';
    END IF;
END $$;

ALTER TABLE accounts
    ADD CONSTRAINT accounts_balance_javascript_safe
    CHECK (balance <= 9007199254740991);

-- +goose Down
ALTER TABLE accounts DROP CONSTRAINT accounts_balance_javascript_safe;
```

- [ ] **Step 4: Guard the atomic SQL update and regenerate sqlc**

Change `AddAccountBalance` in `internal/db/query/accounts.sql` to:

```sql
UPDATE accounts
SET balance = balance + sqlc.arg(amount)
WHERE id = sqlc.arg(id)
  AND balance + sqlc.arg(amount) BETWEEN 0 AND 9007199254740991
RETURNING *;
```

Run:

```bash
mise run sqlc:generate
```

Expected: only generated SQL text changes; the method signature stays stable.

- [ ] **Step 5: Add the domain error and locked-row precheck**

Add to `internal/db/errors.go`:

```go
ErrBalanceLimitExceeded = errors.New("destination balance exceeds supported limit")
```

After `lockAccounts` validates currencies and before creating transfer rows,
add this check in `TransferTx`:

```go
if arg.Amount > currency.MaxSafeMinorUnits ||
	toAccount.Balance > currency.MaxSafeMinorUnits-arg.Amount {
	return ErrBalanceLimitExceeded
}
```

Import `internal/currency`. Row locks make this precheck authoritative for the
transaction; the SQL and schema guards remain defense in depth.

Add this `errorCatalog` row in `internal/api/errors.go`:

```go
{store.ErrBalanceLimitExceeded, http.StatusUnprocessableEntity, "destination balance exceeds the supported limit"},
```

- [ ] **Step 6: Run focused unit and integration tests**

Run:

```bash
go test ./internal/api
mise run compose:test:up
go test -tags=integration ./internal/db -run 'TestTransferTx(AllowsExactSafe|RejectsUnsafe)DestinationBalance' -v
mise run compose:test:down
```

Expected: PASS, with rejected transfer leaving transfer, entry, and account rows
unchanged.

- [ ] **Step 7: Validate generation, formatting, migration, and rollback text**

Run:

```bash
gofmt -w internal/db/errors.go internal/db/transfer_tx.go \
  internal/db/transfer_tx_test.go internal/api/errors.go internal/api/errors_test.go
git diff --check
git diff -- internal/db internal/api/errors.go internal/api/errors_test.go
```

Then run the repository integration task so Goose applies migration `00004` to
a fresh test database:

```bash
mise run test:integration
```

Expected: PASS.

Suggested commit if authorized: `fix(db): cap account balances for JavaScript`

---

### Task 3: Coalesce Concurrent Access-Token Renewal

**Files:**
- Modify: `frontend/src/lib/api/client.ts`
- Modify: `frontend/src/lib/api/client.test.ts`

**Interfaces:**
- Consumes: `auth.tryRefresh(): Promise<boolean>`
- Produces internal: `refreshAccessToken(): Promise<boolean>`
- Preserves: `request<T>(path, options): Promise<T>`

- [ ] **Step 1: Add failing concurrent-renewal tests**

Add this case under `describe("request")`:

```ts
it("shares one refresh across concurrent 401 responses", async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(jsonResponse(401, { error: "expired" }))
    .mockResolvedValueOnce(jsonResponse(401, { error: "expired" }))
    .mockResolvedValueOnce(jsonResponse(200, { id: "a" }))
    .mockResolvedValueOnce(jsonResponse(200, { id: "b" }));
  vi.stubGlobal("fetch", fetchMock);
  const refresh = vi.spyOn(auth, "tryRefresh").mockResolvedValue(true);

  const results = await Promise.all([
    request<{ id: string }>("/accounts/a", { authenticated: true }),
    request<{ id: string }>("/accounts/b", { authenticated: true }),
  ]);

  expect(results).toEqual([{ id: "a" }, { id: "b" }]);
  expect(refresh).toHaveBeenCalledOnce();
  expect(fetchMock).toHaveBeenCalledTimes(4);
});
```

Add a concurrent failed-renewal case expecting one refresh and two fetches, then
a sequential case with two independent `401`/success cycles expecting two
refresh calls.

- [ ] **Step 2: Run focused test and verify RED**

Run:

```bash
cd frontend && bun run test src/lib/api/client.test.ts
```

Expected: concurrent case reports two calls to `auth.tryRefresh`.

- [ ] **Step 3: Implement one shared in-flight promise**

Add near `BASE_URL`:

```ts
let refreshPromise: Promise<boolean> | null = null;

function refreshAccessToken(): Promise<boolean> {
  refreshPromise ??= auth.tryRefresh().finally(() => {
    refreshPromise = null;
  });
  return refreshPromise;
}
```

Replace `await auth.tryRefresh()` in `request` with
`await refreshAccessToken()`. Do not retry more than once and do not make the
renew endpoint authenticated.

- [ ] **Step 4: Run focused test and verify GREEN**

Run:

```bash
cd frontend && bun run test src/lib/api/client.test.ts
```

Expected: PASS.

- [ ] **Step 5: Run frontend type and lint checks**

Run:

```bash
mise run frontend:check
mise run frontend:lint
git diff --check
```

Suggested commit if authorized: `fix(frontend): coalesce session renewal`

---

### Task 4: Prove Router Semantics and Remove State-Sync Effect

**Files:**
- Create: `frontend/src/lib/router.svelte.test.ts`
- Create: `frontend/src/App.test.ts`
- Create: `frontend/src/lib/pages/NewAccountPage.test.ts`
- Modify: `frontend/src/lib/pages/NewAccountPage.svelte`

**Interfaces:**
- Consumes: `router.path`, `navigate(to, state)`, `accounts.load()`
- Preserves: current router exports and all route paths
- Removes: account-opening `$effect` used only to synchronize `currency`

- [ ] **Step 1: Add direct router tests**

Create `frontend/src/lib/router.svelte.test.ts`:

```ts
import { beforeEach, describe, expect, it } from "vitest";
import { navigate, router } from "./router.svelte";

describe("router", () => {
  beforeEach(() => {
    history.replaceState({}, "", "/");
    router.path = "/";
  });

  it("normalizes trailing slashes and stores navigation state", () => {
    navigate("/transfer/", { source: "test" });
    expect(router.path).toBe("/transfer");
    expect(location.pathname).toBe("/transfer");
    expect(history.state).toEqual({ source: "test" });
  });

  it("tracks browser history navigation", () => {
    history.replaceState({}, "", "/accounts/new/");
    window.dispatchEvent(new PopStateEvent("popstate"));
    expect(router.path).toBe("/accounts/new");
  });
});
```

- [ ] **Step 2: Add App title, announcement, and focus test**

Create `frontend/src/App.test.ts`. Use a hoisted auth mock so public routes render
without network work:

```ts
import { cleanup, render, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.svelte";
import { navigate, router } from "./lib/router.svelte";

const authMock = vi.hoisted(() => ({
  initializing: false,
  isAuthenticated: false,
  user: null,
  init: vi.fn(),
}));

vi.mock("./lib/stores/auth.svelte", () => ({ auth: authMock }));

describe("App routing", () => {
  beforeEach(() => {
    history.replaceState({}, "", "/login");
    router.path = "/login";
  });
  afterEach(() => cleanup());

  it("updates title, announces navigation, and focuses main", async () => {
    render(App);
    await waitFor(() => expect(document.title).toBe("Sign in · SimpleBank"));

    navigate("/register");

    await waitFor(() => expect(document.title).toBe("Create account · SimpleBank"));
    expect(document.querySelector('[aria-live="polite"]')).toHaveTextContent("Create account");
    expect(document.querySelector("main")).toHaveFocus();
  });
});
```

- [ ] **Step 3: Add account-load currency-selection regression test**

Create `frontend/src/lib/pages/NewAccountPage.test.ts`. Set `accounts.loaded =
true`, seed one USD account, mock `/account-opening-limits`, render the page, and
wait for the EUR radio to become checked:

```ts
await waitFor(() => {
  expect(screen.getByRole("radio", { name: /EUR/ })).toBeChecked();
});
```

Reset `accounts` and restore globals after the test.

- [ ] **Step 4: Run tests and establish behavior baseline**

Run:

```bash
cd frontend && bun run test \
  src/lib/router.svelte.test.ts src/App.test.ts src/lib/pages/NewAccountPage.test.ts
```

Expected: router and App behavior pass; currency behavior is protected before
moving it out of `$effect`.

- [ ] **Step 5: Move currency correction into initialization**

Replace current `onMount` callback and the currency `$effect` with:

```ts
onMount(() => {
  void initialize();
});

async function initialize(): Promise<void> {
  void loadOpeningLimits();
  if (!accounts.loaded) {
    await accounts.load();
  }
  const firstAvailable = CURRENCIES.find(
    (code) => !accounts.items.some((account) => account.currency === code),
  );
  if (firstAvailable && !available.includes(currency)) {
    currency = firstAvailable;
  }
}
```

Delete only the state-sync `$effect`; retain browser-side effects in `App.svelte`.

- [ ] **Step 6: Run focused tests, full tests, and checks**

Run:

```bash
cd frontend && bun run test \
  src/lib/router.svelte.test.ts src/App.test.ts src/lib/pages/NewAccountPage.test.ts
mise run frontend:test
mise run frontend:check
```

Expected: PASS.

- [ ] **Step 7: Inspect for legacy APIs and malformed diff**

Run:

```bash
rg -n 'on:|createEventDispatcher|<slot|<svelte:component|export let|\$:' frontend/src
git diff --check
```

Expected: no legacy Svelte matches.

Suggested commit if authorized: `test(frontend): cover SPA navigation lifecycle`

---

### Task 5: Establish Tailwind 4 Theme and Application Shell

**Files:**
- Modify generated: `frontend/bun.lock`
- Modify: `frontend/package.json`
- Modify: `frontend/src/main.ts`
- Modify: `frontend/src/app.css`
- Create: `frontend/src/lib/components/Button.test.ts`
- Modify: `frontend/src/lib/components/Button.svelte`
- Modify: `frontend/src/lib/components/Alert.svelte`
- Modify: `frontend/src/lib/components/AppHeader.svelte`
- Modify: `frontend/src/lib/components/AppHeader.test.ts`
- Modify: `frontend/src/lib/components/AppFooter.svelte`
- Modify: `frontend/src/lib/pages/AuthLayout.svelte`

**Interfaces:**
- Adds package imports: `@fontsource-variable/ibm-plex-sans/wght.css` and
  individual `@lucide/svelte/icons/*` modules
- Preserves: `Button`, `Alert`, `AppHeader`, and `AuthLayout` props
- Produces Tailwind tokens: `surface-raised`, `info`, `info-soft`, `attention`,
  `attention-soft`, `font-sans`, `shadow-raised`, and 8px `radius-card`

- [ ] **Step 1: Add behavior tests before changing controls**

Create `Button.test.ts` with a loading-state test using Svelte's typed raw
snippet API:

```ts
import { createRawSnippet } from "svelte";

const children = createRawSnippet(() => ({ render: () => "Save" }));
render(Button, { loading: true, children });
const button = screen.getByRole("button", { name: "Save" });
expect(button).toBeDisabled();
expect(button).toHaveAttribute("aria-busy", "true");
```

Extend `AppHeader.test.ts` to assert menu and close icons are hidden from the
accessibility tree while button names remain `Open navigation` and
`Close navigation`.

- [ ] **Step 2: Run focused tests before dependency and markup changes**

Run:

```bash
cd frontend && bun run test \
  src/lib/components/Button.test.ts src/lib/components/AppHeader.test.ts
```

Expected: accessible-name baseline passes; header's new icon assertion fails
before Lucide conversion.

- [ ] **Step 3: Install only approved dependencies**

Run:

```bash
cd frontend && bun add -d @fontsource-variable/ibm-plex-sans @lucide/svelte
```

Inspect `package.json` and `bun.lock`. Reject any unrelated script or direct
dependency changes.

- [ ] **Step 4: Load font once and define CSS-first theme**

In `frontend/src/main.ts`, add before `./app.css`:

```ts
import "@fontsource-variable/ibm-plex-sans/wght.css";
```

In `app.css`, retain `@import "tailwindcss"`, convert semantic colors to OKLCH,
and add:

```css
@theme {
  --font-sans: "IBM Plex Sans Variable", ui-sans-serif, sans-serif;
  --color-surface-raised: oklch(1 0 0);
  --color-info: oklch(0.47 0.11 246);
  --color-info-soft: oklch(0.95 0.025 246);
  --color-attention: oklch(0.48 0.11 70);
  --color-attention-soft: oklch(0.96 0.04 85);
  --radius-card: 0.5rem;
  --shadow-raised: 0 1px 2px oklch(0.2 0.02 250 / 0.08);
}
```

Set `body` to `font-family: var(--font-sans)` and keep current global focus and
reduced-motion rules. Add forced-colors styles only where native colors would
otherwise erase a border or status distinction.

- [ ] **Step 5: Convert standard shell/control icons using direct imports**

Use tree-shaken imports documented by Lucide:

```ts
import LoaderCircle from "@lucide/svelte/icons/loader-circle";
import Menu from "@lucide/svelte/icons/menu";
import X from "@lucide/svelte/icons/x";
import LogOut from "@lucide/svelte/icons/log-out";
```

Replace the Button spinner with `<LoaderCircle aria-hidden="true" size={16}
class="animate-spin motion-reduce:animate-none" />`. Replace only the menu,
close, and sign-out symbols in `AppHeader`; retain the custom bank mark in the
header and `AuthLayout`.

Refine classes without changing component contracts:

- active navigation gets a visible current marker plus existing `aria-current`;
- sign out retains visible text with `LogOut` icon;
- `Alert` maps info to `info-soft/info`, adds a border, and keeps role behavior;
- footer density and contrast remain secondary;
- auth layout keeps one un-nested form card with 8px radius and restrained
  elevation.

- [ ] **Step 6: Run focused tests and static gates**

Run:

```bash
cd frontend && bun run test \
  src/lib/components/Button.test.ts src/lib/components/AppHeader.test.ts
mise run frontend:check
mise run frontend:lint
mise run frontend:format:check
mise run frontend:build
```

Expected: PASS; build output contains local font assets and no remote font URL.

- [ ] **Step 7: Inspect dependency and CSS scope**

Run:

```bash
git diff -- frontend/package.json frontend/bun.lock frontend/src/main.ts \
  frontend/src/app.css frontend/src/lib/components frontend/src/lib/pages/AuthLayout.svelte
git diff --check
```

Confirm only two direct packages were added and no `tailwind.config.js` exists.

Suggested commit if authorized: `style(frontend): refine theme and app shell`

---

### Task 6: Refine Dashboard and Account Activity

**Files:**
- Modify: `frontend/src/lib/pages/DashboardPage.svelte`
- Modify: `frontend/src/lib/components/AccountCard.svelte`
- Modify: `frontend/src/lib/components/AccountCard.test.ts`
- Modify: `frontend/src/lib/pages/AccountHistoryPage.svelte`

**Interfaces:**
- Preserves: account card `account: Account` prop, routes, copy action, transfer
  preselection, and activity data flow
- Uses icons: `copy`, `check`, `send`, `history`, `arrow-up-right`, and
  `arrow-down-left` from direct `@lucide/svelte/icons/*` imports

- [ ] **Step 1: Strengthen AccountCard interaction assertions**

Extend `AccountCard.test.ts` so copy, send, and activity controls retain visible
names after icons are introduced. Mock `navigator.clipboard.writeText`, click
Copy, and assert the name changes to `Account number copied` without exposing
the icon to the accessibility tree.

- [ ] **Step 2: Run focused test before markup changes**

Run:

```bash
cd frontend && bun run test src/lib/components/AccountCard.test.ts
```

Expected: existing behavior passes; new icon-hiding assertion fails before
conversion.

- [ ] **Step 3: Refine dashboard hierarchy**

Keep summary as a full-width band, not a card inside another card. Apply:

- `rounded-card` with the new 8px token;
- responsive action wrapping and full-width buttons at 320px;
- stable tabular totals with no viewport-scaled typography;
- account count as secondary metadata;
- current loading, error, empty, and unverified-email branches unchanged.

Use `Send` and `Plus` icons in dashboard commands with `aria-hidden="true"`.
Do not add charts or combine currencies into one total.

- [ ] **Step 4: Refine account cards**

Use direct Lucide imports. Keep account number selectable, keep Copy text, and
replace the arrow character in `Send money →` with a `Send` icon. Use
`shadow-raised` only if border separation is insufficient on canvas. Ensure
actions wrap without overlap at 320px and technical IDs remain monospace.

- [ ] **Step 5: Refine account activity list**

Keep semantic `<ul>/<li>` markup. Add direction icons with hidden decorative
SVGs, allow counterparty IDs to wrap rather than combine `truncate` with
`break-all`, align signed tabular amounts, and preserve text labels `Sent` and
`Received` so color and icons are not sole status indicators.

- [ ] **Step 6: Run component and frontend gates**

Run:

```bash
cd frontend && bun run test src/lib/components/AccountCard.test.ts
mise run frontend:test
mise run frontend:check
mise run frontend:lint
```

Expected: PASS.

- [ ] **Step 7: Inspect this presentation-only diff**

Run:

```bash
git diff -- frontend/src/lib/pages/DashboardPage.svelte \
  frontend/src/lib/components/AccountCard.svelte \
  frontend/src/lib/components/AccountCard.test.ts \
  frontend/src/lib/pages/AccountHistoryPage.svelte
git diff --check
```

Confirm no API calls, route names, or store mutations changed.

Suggested commit if authorized: `style(frontend): improve account scanning`

---

### Task 7: Refine Forms and Public States

**Files:**
- Modify: `frontend/src/lib/pages/TransferPage.svelte`
- Modify: `frontend/src/lib/pages/NewAccountPage.svelte`
- Modify: `frontend/src/lib/pages/LoginPage.svelte`
- Modify: `frontend/src/lib/pages/RegisterPage.svelte`
- Modify: `frontend/src/lib/pages/VerifyEmailPage.svelte`
- Modify: `frontend/src/lib/pages/NotFoundPage.svelte`
- Modify: `frontend/src/lib/components/TextField.svelte`
- Modify: `frontend/src/lib/components/TextField.test.ts`
- Modify: `frontend/src/lib/pages/NewAccountPage.test.ts`

**Interfaces:**
- Preserves: all form submission payloads, validation messages, history state,
  first-invalid-field focus, and account policy behavior
- Uses direct icons: `arrow-left`, `circle-check`, `circle-alert`, and
  `loader-circle`

- [ ] **Step 1: Strengthen field and account-form tests**

In `TextField.test.ts`, assert error state retains `aria-invalid`, error-first
`aria-describedby`, and disabled state semantics.

In `NewAccountPage.test.ts`, retain the loaded-account currency test from Task 4
and assert the currency fieldset exposes `aria-busy` while policy loading is
active.

- [ ] **Step 2: Run focused tests before styling changes**

Run:

```bash
cd frontend && bun run test \
  src/lib/components/TextField.test.ts src/lib/pages/NewAccountPage.test.ts
```

Expected: semantic assertions pass and establish behavior baseline.

- [ ] **Step 3: Standardize transfer and account-opening composition**

Keep each page at `max-w-lg`. Replace back arrow text with an `ArrowLeft` icon
plus visible `Back`. Use the same heading, description, form-gap, action-width,
and receipt/policy spacing on both pages. Mobile submit buttons use `w-full`.

Keep receipt content as one genuine framed result. Do not wrap the whole form in
another card. Keep native select, radio, and number inputs.

- [ ] **Step 4: Refine public and status pages without changing copy flow**

Keep Login and Register behavior unchanged; align form spacing and button width
through existing controls. In `VerifyEmailPage`, replace hand-drawn status SVGs
and spinner with direct Lucide components. In `NotFoundPage`, retain literal 404
heading structure and add no marketing content.

- [ ] **Step 5: Refine field environmental states**

Keep `$props.id()`, `$bindable`, focus-on-first-new-error effect, and semantic
labels. Add Tailwind classes for disabled, contrast-more, and forced-colors only
where they reinforce native state. Do not rely on placeholder text as a label
and do not dynamically interpolate utility names.

- [ ] **Step 6: Run focused and full frontend verification**

Run:

```bash
cd frontend && bun run test \
  src/lib/components/TextField.test.ts src/lib/pages/NewAccountPage.test.ts
mise run frontend:test
mise run frontend:check
mise run frontend:lint
mise run frontend:format:check
mise run frontend:build
```

Expected: PASS.

- [ ] **Step 7: Inspect workflow preservation and legacy syntax**

Run:

```bash
rg -n 'on:|createEventDispatcher|<slot|<svelte:component|export let|\$:' frontend/src
git diff --check
git diff -- frontend/src/lib/pages frontend/src/lib/components/TextField.svelte
```

Expected: no legacy API matches and no request payload changes.

Suggested commit if authorized: `style(frontend): unify form workflows`

---

### Task 8: Expand Browser Proof and Run Final Gates

**Files:**
- Modify: `frontend/e2e/accessibility.spec.ts`
- Create after review: Playwright snapshots under
  `frontend/e2e/accessibility.spec.ts-snapshots/` only if screenshot assertions
  are accepted for this repository
- Modify if required for deterministic screenshots: `frontend/playwright.config.ts`

**Interfaces:**
- Consumes: unchanged routes and mocked `/api/v1` responses
- Proves: supported viewports, browser history, font loading, account activity,
  no overflow, no Axe violations, no runtime errors, and stable screenshots

- [ ] **Step 1: Extend API mocks for account activity**

Add deterministic routes for one account and its transfers:

```ts
await page.route(`**/api/v1/accounts/${account.id}`, (route) =>
  route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(account) }),
);
await page.route(`**/api/v1/accounts/${account.id}/transfers?*`, (route) =>
  route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify([
      {
        id: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
        from_account_id: account.id,
        to_account_id: "99999999-8888-7777-6666-555544443333",
        amount: 25_00,
        idempotency_key: "11111111-aaaa-bbbb-cccc-222222222222",
        created_at: "2026-01-16T10:00:00Z",
      },
    ]),
  }),
);
```

Register the more specific transfer route before any broad account route that
could intercept it.

- [ ] **Step 2: Add history, activity, font, and runtime assertions**

Add tests that:

1. navigate Dashboard to Transfer through visible UI, call `page.goBack()`, and
   assert Dashboard title and focused `main`;
2. open account Activity at 320 and 1440px, assert Sent text and signed amount,
   then check `scrollWidth <= clientWidth` and Axe;
3. assert local font availability:

```ts
expect(
  await page.evaluate(() => document.fonts.check('16px "IBM Plex Sans Variable"')),
).toBe(true);
```

4. collect `page.on("console")` error messages and `page.on("requestfailed")`
   URLs before navigation, then assert both arrays remain empty.

- [ ] **Step 3: Run E2E tests and verify new assertions**

Run:

```bash
mise run frontend:test:e2e
```

Expected: any incorrect selector, wrapping, font loading, or focus behavior fails
with a specific assertion. Repair only task-related defects.

- [ ] **Step 4: Capture deterministic mobile and desktop baselines**

After functional E2E tests pass, add screenshot assertions for the populated
dashboard at 320x800 and 1440x1000:

```ts
await expect(page).toHaveScreenshot(`dashboard-${viewport.width}.png`, {
  animations: "disabled",
  fullPage: true,
});
```

Generate initial snapshots once:

```bash
cd frontend && bun run test:e2e --update-snapshots
```

Open both images and inspect typography, clipping, overlap, balance alignment,
actions, and next-section visibility. Keep snapshots only after human-visible
inspection; otherwise remove screenshot assertions and record manual screenshots
during execution.

- [ ] **Step 5: Run complete frontend gates**

Run:

```bash
mise run frontend:test
mise run frontend:check
mise run frontend:lint
mise run frontend:format:check
mise run frontend:build
mise run frontend:test:e2e
```

Expected: all PASS.

- [ ] **Step 6: Run complete Go and repository gates**

Run:

```bash
mise run test:unit
mise run test:integration
mise run golangci-lint
```

Expected: all PASS. Do not fix unrelated failures; report them separately with
the failing command and shortest decisive output.

- [ ] **Step 7: Final deprecated-pattern and scope scan**

Run:

```bash
rg -n 'on:|createEventDispatcher|<slot|<svelte:component|SvelteComponent|ComponentType|theme\(' frontend/src
find frontend -maxdepth 2 -name 'tailwind.config.*' -print
git diff --check
git status --short
```

Expected: no deprecated Svelte/Tailwind matches, no Tailwind config file, clean
diff whitespace, and only planned files changed.

- [ ] **Step 8: Review final diff across five quality axes**

Review correctness, readability, architecture, security, and performance. Pay
special attention to concurrent renewal settlement, transaction rollback,
keyboard focus, generated SQL, dependency lock changes, and mobile text fit.
Use cavecrew reviewer for concise findings or the full code-reviewer agent when
rationale and alternatives are needed. Address required findings and rerun the
smallest falsifying check before repeating final gates.

Suggested final commit if authorized: `test(frontend): verify responsive SPA workflows`
