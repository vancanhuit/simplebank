# Final Fix Wave

## Changes

- Added runtime shape validation for login and renewal success payloads before auth state mutation.
- Moved generation-scoped refresh coalescing into `AuthStore`; initialization, explicit retry, and automatic 401 recovery now use the same in-flight promise.
- Removed the API client's duplicate refresh coordinator.
- Added direct retained-data coverage for Dashboard account cards and New Account form values.

## TDD Evidence

RED command:

```text
mise run frontend:test -- src/lib/stores/auth.svelte.test.ts src/lib/api/client.test.ts src/lib/pages/DashboardPage.test.ts src/lib/pages/NewAccountPage.test.ts
```

Initial result: 9 failures. The auth failures showed malformed login payloads resolving without error, malformed renewals returning `refreshed`, distinct refresh promises, and two `/tokens/renew` requests in both manual/automatic orderings. Two page-test assertion fixture issues were corrected before production edits; the direct page behavior then passed unchanged.

GREEN focused result:

```text
Test Files  4 passed (4)
Tests       89 passed (89)
```

## Final Verification

```text
mise run frontend:test
Test Files  31 passed (31)
Tests       271 passed (271)

mise run frontend:check
svelte-check found 0 errors and 0 warnings

mise run frontend:lint
exit 0

mise run frontend:format:check
All matched files use Prettier code style!

Svelte autofixer: frontend/src/lib/stores/auth.svelte.ts
issues: 0; suggestions: 0

git diff --check
exit 0
```

## Files

- `frontend/src/lib/stores/auth.svelte.ts`
- `frontend/src/lib/stores/auth.svelte.test.ts`
- `frontend/src/lib/api/client.ts`
- `frontend/src/lib/api/client.test.ts`
- `frontend/src/lib/pages/DashboardPage.test.ts`
- `frontend/src/lib/pages/NewAccountPage.test.ts`
- `.superpowers/sdd/2026-08-24-user-facing-error-handling/final-fix-report.md`

## Invariant Review

- Auth credentials are assigned only after complete runtime shape validation.
- Invalid successful login payloads surface classified `invalid_response` errors and do not mutate credentials.
- Invalid successful renewal payloads preserve existing credentials, set `renewalUnavailable`, and return `unavailable`.
- At most one renewal request is active per auth generation, including initialization, manual retry, and automatic 401 recovery.
- A stale generation cannot apply renewal state, and completion of an older attempt cannot clear a newer generation's coordinator.
- Definitive `204`/`401` invalidation behavior and session-expired semantics remain unchanged.
- Cached account cards and failed account-opening input remain visible for recovery.
