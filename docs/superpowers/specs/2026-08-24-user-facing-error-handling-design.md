# User-Facing Error Handling Design

## Goal

Make errors safe, consistent, calm, actionable, and accessible across the Go API and Svelte SPA. Preserve user work and authenticated state through temporary failures, while retaining the existing authorization, session-generation, notification rollback, and transfer-idempotency invariants.

## Current Problems

- The frontend displays arbitrary backend and browser `Error.message` values.
- HTTP status alone cannot distinguish several conflicts and validation failures.
- A network or server failure during token renewal clears authentication as if the session were invalid.
- Error wording, alert semantics, retry actions, and ownership vary between pages.
- Render and effect failures can remove the application without a safe recovery screen.

## API Error Contract

Non-success JSON responses use this shape:

```json
{
  "code": "insufficient_balance",
  "error": "insufficient balance"
}
```

`code` is a stable, machine-readable identifier. `error` remains a client-safe fallback for API consumers, but the SPA does not pass arbitrary server text directly to users. The contract does not include a request or support reference in the UI.

The central API error catalog owns status, code, and safe fallback message for domain errors. It includes distinct codes where frontend behavior or guidance differs, including username/email conflicts, balance and transfer limits, currency mismatch, idempotency conflict, invalid session, and invalid or expired tokens.

Explicit handler errors use a typed API helper or catalog entry rather than unrestricted `echo.HTTPError` text at the response boundary. Echo framework statuses receive generic stable codes such as `not_found`, `method_not_allowed`, and `unauthorized`. Unknown errors return `500`, `internal_error`, and a generic message; internal details remain available only to request logging.

Malformed request bodies and payload validation remain trust-boundary checks. They receive distinct stable codes but do not expose validator internals. Existing HTTP statuses remain unchanged unless a test reveals an incorrect status.

## Frontend Error Model

The API client owns classification. Its error type records:

- kind: API, network, malformed response, or aborted request
- HTTP status when available
- stable API code when recognized
- retry delay for rate limiting when available

Fetch rejections and malformed successful responses never expose their native messages. Aborted requests remain silent in existing stale-response and route-cleanup flows. A single formatter maps classified failures to calm, actionable defaults such as:

- Network: "We couldn't reach SimpleBank. Check your connection and try again."
- Server: "SimpleBank is temporarily unavailable. Please try again."
- Rate limit: "Too many attempts. Try again in 30 seconds."
- Unknown: "Something went wrong. Please try again."

Known domain codes receive specific copy. Pages may add operation context, such as "We couldn't load your accounts," but do not inspect raw browser errors or duplicate status/code switches. Existing local field validation remains inline and continues to focus the first invalid field.

## Session And Navigation Behavior

Refresh outcomes have explicit semantics:

- `200`: apply the rotated access token and user if the auth generation is still current.
- `204`: no refresh cookie exists; clear auth and treat the browser as signed out.
- `401`: the refresh session is definitively invalid or expired; clear auth and redirect to login with a one-shot session-expired notice.
- Network or `5xx`: preserve the current in-memory user and token, mark renewal temporarily unavailable, and allow an explicit retry. Do not redirect or clear session-scoped stores.

On initial application load there is no in-memory user to preserve. A temporary refresh failure produces a startup recovery screen with retry rather than incorrectly presenting the login page. A definitive `204` or `401` proceeds to login.

When an authenticated request receives `401`, the client still coalesces one refresh attempt and retries the request once. A temporary refresh failure returns a classified session-unavailable error to the caller and keeps auth state. A definitive refresh rejection clears auth once. Generation checks continue preventing pre-logout responses from repopulating a later session.

Protected-route redirects use history replacement. The login navigation state stores only a validated internal return path. Successful login returns to that path; otherwise it opens the dashboard. This avoids back-button redirect loops and restores the interrupted task.

## Presentation And Recovery

The shared `Alert` component remains the standard inline presentation and keeps `role="alert"` for errors. Notification mutation errors use it instead of hand-built alert markup. Stores own request concurrency, stale-response protection, optimistic rollback, and retryable operation state; pages own placement and action-specific context.

Recoverable load failures retain already visible data where current behavior does so and provide a retry action. Submission failures preserve entered values and allow resubmission. Transfer failures continue preserving the idempotency key for an unchanged retry so an uncertain response cannot duplicate a debit.

A persistent application-level connectivity/session-renewal alert appears when the current authenticated session cannot be renewed temporarily. Its retry action attempts renewal and then refreshes affected data through existing store mechanisms. Browser offline state may improve the message, but `navigator.onLine` is advisory; actual request outcomes remain authoritative.

The application content is wrapped in a Svelte error boundary. Rendering or effect failures show a sanitized fallback with "Try again" and page reload actions. The original error may be logged to the console in development, but its message is not rendered. Event-handler and asynchronous request errors remain handled by their existing explicit paths because Svelte boundaries do not catch them.

Notifications retain their durable list and SSE reconnection behavior. Stream disconnection alone does not generate repetitive alerts; failed reconciliation exposes one retryable stale-data message through the existing notification store.

## Scope And Ownership

Expected backend changes are concentrated in `internal/api/errors.go`, the validation/handler call sites that currently create free-form HTTP errors, and their API tests.

Expected frontend changes are concentrated in:

- `frontend/src/lib/api/client.ts` for typed classification and safe messages
- `frontend/src/lib/stores/auth.svelte.ts` for refresh outcomes and transient recovery state
- `frontend/src/App.svelte` and routing for startup recovery, session notices, return paths, and the root boundary
- existing pages/components for operation context, consistent `Alert` use, and retry actions

No new dependency, error service, telemetry system, localization framework, or speculative extension point is introduced.

## Testing

Backend tests cover every catalog entry, generic framework statuses, explicit safe errors, malformed/invalid payloads, and unknown-error redaction. Existing endpoint tests are updated for the additive `code` field.

Frontend unit and component tests cover:

- API-code parsing and safe handling of unknown server text
- network, malformed-response, abort, rate-limit, and generic `5xx` classification
- refresh `200`, `204`, `401`, network, and `5xx` outcomes with generation races
- startup retry versus signed-out rendering
- transient renewal preserving auth and definitive rejection clearing session stores
- return-path validation and replace navigation
- consistent notification alert semantics and retry behavior
- sanitized root-boundary fallback and reset/reload controls
- representative login, registration, account, transfer, verification, and load failure messages

The focused frontend suite runs during iteration. Completion uses the repository frontend checks, lint, format check, unit tests, and Playwright tests; backend changes also use Go formatting, lint, and unit tests.

## Non-Goals

- Field-level backend validation details beyond current safe local validation
- Automatic retries for money-moving requests
- Displaying request IDs or internal diagnostics to users
- Adding monitoring, localization, or offline/PWA support
- Reworking successful states or the existing visual design
