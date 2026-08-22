# Svelte Frontend Correctness Hardening Design

## Context

The SimpleBank frontend is a Svelte 5 SPA with TypeScript, Tailwind CSS,
daisyUI, Vitest, Playwright, and axe. Its existing quality gates pass, and an
official Svelte MCP audit found no compiler, rune, lint, formatting, unit-test,
or accessibility failures. The useful improvements are therefore behavioral
hardening rather than broad modernization or visual refactoring.

The current frontend already uses monotonic generations to stop asynchronous
authentication and account responses from repopulating state after logout.
This design extends that established pattern to close uncovered session,
request-ordering, and mutation-intent races while keeping the current SPA
architecture.

## Goals

- Prevent an authenticated request started by one session from being retried
  with credentials from a later session.
- Prevent an older account load from overwriting a newer same-session result.
- Reuse a transfer idempotency key only for an identical transfer intent.
- Keep account-dependent forms unavailable until account inventory is known.
- Preserve logout failure feedback after authenticated chrome unmounts.
- Add accessible client-side login and registration validation that mirrors
  stable backend constraints while leaving the backend authoritative.
- Prove each behavior with focused tests and all existing frontend quality
  gates.

## Non-Goals

- A visual redesign or changes to the daisyUI theme.
- General component extraction or optional style cleanup.
- Replacing the router, API client, or state classes.
- Introducing a request coordinator, dependency, interface, or service layer.
- Adopting experimental async Svelte or optional `$state.raw` optimization.
- Changing backend APIs, authentication policy, or transfer semantics.

## Architecture

The existing modules continue to own their current responsibilities:

- `AuthStore` owns the authenticated session identity and its monotonic
  generation.
- `api/client.ts` owns token attachment, refresh coalescing, and retry policy.
- `AccountsStore` owns the session-scoped account cache and load lifecycle.
- Pages own form intent, field validation, and presentation of store state.
- `App.svelte` remains the composition point for auth routing and account reset.

No new architectural layer is required. The hardening extends these owners with
the minimum additional state needed to express each invariant.

## Session-Bound Requests

`AuthStore` will expose its current generation through a read-only getter. The
generation identifies the local auth epoch, not a user or token, and remains
non-reactive because it is used for asynchronous consistency checks rather than
rendering.

Every authenticated `request` captures the generation before its first send.
On a 401, the client may refresh and retry only when that generation is still
current. Refresh coalescing will also be scoped to the captured generation so a
request from a new session cannot join a refresh initiated by an old session.

If the generation changes before refresh or retry, the request decodes its
original 401 response. It must not refresh, attach the new session's access
token, or repeat a mutation. This fail-closed behavior prevents a stale account
creation or transfer from executing as the next signed-in user.

Generation changes must cover login attempts, logout, and any operation that
invalidates the current auth epoch. State clearing that belongs to the same
already-advanced operation must not accidentally create extra semantic epochs;
the implementation will centralize generation advancement in the relevant
auth transition methods.

## Ordered Account Loading

`AccountsStore` keeps its existing generation, which protects session resets,
and adds a separate monotonically increasing load sequence. Each `load` captures
both values.

Only a load whose session generation is unchanged and whose sequence is still
the latest may update `items`, `error`, `loaded`, or `loading`. Therefore:

- An old session can never repopulate a new session.
- An older same-session response cannot replace fresher account data.
- Completion of an older request cannot clear the loading indicator while a
  newer load is pending.

The store will continue to expose a single latest-load state rather than track
all in-flight requests. Superseded requests may finish normally; their results
are ignored.

## Transfer Intent And Idempotency

The transfer page will associate each idempotency key with the normalized
immutable intent submitted to the API:

- source account ID
- trimmed recipient account ID
- amount in minor units
- currency

Before submission, the page compares the validated intent with the intent bound
to the current key. An identical retry reuses the key, preserving protection
against a committed transfer whose response was lost. A changed intent receives
a new key before the request is sent, avoiding the backend's expected conflict
for reusing a key with different transfer details.

After confirmed success, both the remembered intent and key are reset for the
next transfer. Invalid client-side submissions do not consume or bind a key
because no API attempt occurred.

## Account-Dependent Pages

Transfer and account-creation forms require an authoritative account inventory.
They will render store-driven states in this order:

1. While inventory is unknown and loading, show an accessible loading state.
2. If loading failed, show `accounts.error` and a retry action.
3. Once loaded, show the existing empty state or the actionable form.

`NewAccountPage` will coordinate account inventory independently from the
account-opening policy. Both must be ready before account creation is enabled.
A failed account load cannot be interpreted as an empty account list and cannot
expose currencies that may already be held.

`TransferPage` likewise cannot expose an enabled empty form while account data
is loading or unavailable. Transfer-limit policy remains non-fatal because the
backend enforces it and the inventory requirement is independent.

## Logout Failure Feedback

Logout will invalidate local credentials before waiting for the server request,
so a usable access token does not remain available during sign-out. Navigation
to login still occurs regardless of the server result.

Because the authenticated header unmounts after local invalidation, it cannot
own durable failure presentation. The existing `navigate` history-state
argument will carry a `logoutFailed` flag to the login route. The login page
reads that flag and immediately removes it from the current history entry with
`history.replaceState`, making the notice one-shot even across later browser
history navigation. The notice explains that local sign-out succeeded but the
server logout request failed, without implying that the user remains signed in.

The notice is ephemeral, contains no sensitive data, and does not persist in
browser storage.

## Authentication Form Validation

Login and registration will continue to use explicit submit handlers and
backend errors. Client validation will mirror only stable backend constraints:

- login requires a non-empty alphanumeric username after trimming and a
  non-empty password
- registration requires a non-empty full name, username, email, and password
- registration trims full name, username, and email before validation and
  submission
- registration requires an alphanumeric username and valid email syntax
- registration requires a password of at least 15 characters and at most 72
  UTF-8 bytes

Validation errors will be field-specific, connected to inputs through the
existing `TextField` accessibility behavior, and clear when the relevant input
changes. Invalid forms will not call the API. Backend validation remains the
trust boundary and its safe messages remain the fallback for constraints the
client cannot or should not duplicate.

## Error Handling

- A stale authenticated request returns its original decoded API error and is
  never silently transferred to another session.
- The latest account load owns its error and loading state; superseded loads are
  silent.
- Account inventory failures are retryable from each dependent page.
- Transfer failures retain their intent-bound key for an identical retry.
- Logout failures do not restore local credentials.
- No mutation updates optimistic account state before server confirmation.

## Testing

Tests will be added before their corresponding implementation changes.

### API And Authentication

- A delayed 401 followed by logout and a new login is not retried.
- A new auth generation does not join an old generation's refresh promise.
- Same-generation concurrent 401 responses still coalesce refresh correctly.
- Logout clears local credentials before the server request settles.
- An integrated app test verifies that logout failure feedback appears on login
  and is consumed once.

### Account Store

- Two same-session loads resolving newest-first retain the newer result.
- The older load cannot clear loading while the newer load is pending.
- Existing reset-generation tests continue to prove cross-session isolation.

### Transfer Page

- An unchanged retry reuses its idempotency key.
- Changing source, recipient, amount, or currency rotates the key.
- Invalid local submissions do not bind a new intent.
- Loading and account-load failure states hide or disable the form and expose a
  working retry action.

### Account Creation And Authentication Forms

- Account creation is unavailable until account inventory and opening policy
  both load successfully.
- Account-load failure exposes retry and cannot be treated as no accounts.
- Login and registration reject invalid fields without calling the API.
- Validation errors are associated with their controls and clear on correction.

## Verification

Every changed `.svelte` file will be checked with the official Svelte MCP
autofixer until it reports no actionable issues. The completed change must pass:

```sh
mise run frontend:check
mise run frontend:lint
mise run frontend:format:check
mise run frontend:test
mise run frontend:test:e2e
```

Playwright's existing desktop/mobile and axe checks remain the browser-level
accessibility gate. Axe coverage will include registration validation and the
account-loading failure states at a representative mobile viewport; detailed
state transitions remain component-test responsibilities.

## Acceptance Criteria

- No authenticated request can refresh or retry across an auth generation.
- Only the newest same-session account load can publish state.
- Transfer idempotency keys are stable for identical retries and rotate for
  changed intent.
- Transfer and account-creation forms are unavailable when account inventory is
  loading or failed.
- Logout invalidates credentials immediately and reports server failure after
  navigation.
- Login and registration prevent known-invalid submissions accessibly.
- All Svelte MCP checks and frontend completion gates pass.
