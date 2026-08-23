# Task 12 Report

## Status

Implemented the protected notification history route, authenticated notification lifecycle, persistent toast placement, and account-activity invalidation refresh.

## RED / GREEN

- RED: notification history tests failed because the page did not exist.
- GREEN: history loading/empty/retained-error states, cursor pagination, mark-all, and mark-before-navigation pass.
- RED: App tests failed because notification lifecycle, route metadata, guard integration, and persistent toasts were absent.
- GREEN: authenticated startup is once per auth generation; sign-out and unmount reset resources; `/notifications` is guarded, titled, and announced.
- RED: account history tests failed because activity versions were not observed, requests lacked cancellation, and refreshes cleared successful data.
- GREEN: affected-account versions refresh account and transfers; same-route refresh preserves data and reports compact errors; route changes clear; abort/load/auth generations reject stale work.

## Lifecycle

- `App` starts notifications once for each authenticated auth generation, without restarting for access-token replacement.
- Signed-out resolution resets notifications and accounts.
- Root unmount resets notification stream, timers, listeners, and state through the store owner.
- Persistent notification toasts mount only inside authenticated chrome.
- Account activity effects own an `AbortController`; every result path checks request generation, auth generation, and abort state.

## Svelte Autofixer

- `NotificationsPage.svelte`: clean, no issues or suggestions.
- `AccountHistoryPage.svelte`: no issues; expected advisory suggestions remain for the required reactive network effect and its state updates.
- `App.svelte`: no issues; expected advisory suggestions remain for established lifecycle/routing effects and route-announcement state.

## Verification

- Focused tests: 20 passed.
- Full frontend unit/component suite: 28 files, 185 tests passed.
- Svelte/TypeScript check: 0 errors, 0 warnings.
- ESLint: passed.
- Prettier check: passed.
- Playwright: 15 passed; authenticated fixtures now mock notification history/stream, aborted navigation requests are treated as intentional cancellation, and snapshots include the notification bell.

## Concerns

- None material. Autofixer advisories are false positives for effects whose purpose is external lifecycle, routing, focus, and cancellable fetching.

## Fix Round 1/5

- RED: account-history regression showed changing account B's activity invalidated account A; initial-failure retry regression showed a blank state instead of loading.
- GREEN: notification activity versions now use per-account reactive holder instances, so only the affected account invalidates. Reset increments existing holders before clearing them so active readers leave the old session safely.
- GREEN: account history tracks the route of the last successful load. Retries without successful data use the initial loading state; retained-data retries continue preserving visible data.
- Focused proof: `mise run frontend:test -- src/lib/stores/notifications.svelte.test.ts src/lib/pages/AccountHistoryPage.test.ts src/lib/pages/NotificationsPage.test.ts src/App.test.ts` — 4 files, 36 tests passed.
- Autofixer: `AccountHistoryPage.svelte` had no issues; only the expected advisory for its required cancellable network effect.
- Quality gates: `mise run frontend:check`, `mise run frontend:lint`, and `mise run frontend:format:check` all passed; Svelte check reported 0 errors and 0 warnings.
