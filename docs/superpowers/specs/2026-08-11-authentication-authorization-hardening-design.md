# Authentication and Authorization Hardening Design

## Status

Approved for implementation on 2026-08-11.

## Context

SimpleBank is a demo banking application. Users intentionally seed newly created
accounts with demo funds, but that capability needs a bounded, per-currency
opening balance. The remaining authentication and authorization audit findings
must be fixed:

- refresh JWTs currently authorize protected routes as bearer access tokens;
- transfer idempotency replay can return another user's account data;
- browser-readable refresh tokens survive client-only logout and do not rotate;
- unverified users receive full financial access;
- registration reveals whether an email address already exists.

This design keeps the application simple while making credential purpose,
session lifecycle, resource ownership, and externally visible responses
explicit.

## Goals

- Enforce distinct access-token and refresh-token purposes.
- Store refresh credentials only in secure, HttpOnly cookies.
- Rotate refresh credentials once per successful renewal and revoke them on
  logout.
- Scope transfer idempotency to the authorized source account and reject key
  reuse with different transfer parameters.
- Block login until email verification completes.
- Hide email-address existence during registration and verification resend.
- Preserve intentional demo funding with configurable per-currency caps.
- Keep access tokens short-lived and in memory only.

## Non-Goals

- User-facing session management across devices.
- Refresh-token family history or descendant revocation beyond the single
  stable active session.
- Roles beyond the existing depositor role.
- Password reset, account recovery, or multi-factor authentication.
- Removing demo opening balances.

## Token Purpose

JWT payloads gain a required `token_type` claim with exactly two values:
`access` and `refresh`. Token creation requires a type, and verification requires
the expected type. A token with a valid signature and wrong purpose is invalid.

The protected-route middleware constructs claims expecting `access`. The renew
and logout handlers verify cookies expecting `refresh`. Both token kinds retain
the existing subject, role, UUID, issued-at time, and expiry. HS256 remains
allowlisted, and the existing minimum secret length remains in force.

This closes the refresh-as-bearer bypass without adding another signing key.
Separate keys can be introduced later if operational isolation becomes useful.

## Refresh Cookie And Session Lifecycle

Login returns the access token, access expiry, session ID, and user profile as
JSON. It no longer returns the refresh token. Instead, it sets a refresh cookie
with these properties:

- `HttpOnly` enabled;
- `SameSite=Strict`;
- `Secure` controlled by explicit `SESSION_COOKIE_SECURE` configuration and
  enabled by default;
- path `/api/v1` so renew and logout receive it;
- expiry aligned with the refresh-token expiry.

Plain HTTP development profiles explicitly set `SESSION_COOKIE_SECURE=false`.
Production-like HTTPS profiles keep the secure default. Configuration
validation rejects an insecure cookie when `PUBLIC_BASE_URL` uses HTTPS.

`POST /api/v1/tokens/renew` takes no token body. It reads the cookie, verifies
refresh purpose, and runs one database transaction:

1. Lock the session row by the JWT ID.
2. Validate username, token hash, blocked state, and expiry.
3. Mint a new access token and refresh token.
4. Preserve the stable session/JWT ID and replace the refresh hash and expiry
  with the new refresh token's values.
5. Commit, set the replacement cookie, and return the access token, expiry, and
  current user profile so the SPA can restore cookie-backed sessions after a
  reload without persisting profile data in browser storage.

The old refresh token no longer matches the stored hash after commit, making
every refresh token single-use. Concurrent renewals serialize on the stable
session row; one succeeds and the others receive `401`. Logout blocks that same
stable row, so it cannot miss a replacement created by an in-flight renewal.

`POST /api/v1/users/logout` verifies and blocks the current session when a valid
cookie is present, then expires the cookie. It returns `204` even when the cookie
is absent or already invalid, making logout idempotent without exposing session
state.

The SPA removes the legacy `simplebank.session` localStorage item during
initialization. Existing browser sessions must log in once after deployment.
Access tokens remain memory-only. Logout calls the server and clears local state
even if the network request fails.

## Transfer Idempotency Authorization

The transfer uniqueness constraint changes from global `idempotency_key` to
`(from_account_id, idempotency_key)`. Lookup and concurrent-conflict replay use
both values.

Before replaying an existing transfer, the store compares its destination and
amount with the current request. Currency remains validated against account
rows. Any mismatched immutable parameter returns a stable idempotency conflict,
mapped to HTTP `409`.

The API continues authorizing ownership of the caller-supplied source account
before entering the transaction. Because replay lookup is scoped to that same
source account, a recipient cannot use a visible key to retrieve the original
sender's account objects.

## Email Verification And Registration Privacy

Login performs password verification first, preserving uniform invalid-login
behavior. When credentials are valid but `is_email_verified` is false, login
returns `403` with a verification-required message and issues no tokens or
session.

Registration behavior becomes:

- a new username and email create the user and enqueue verification mail;
- an existing username returns `409` so users can choose another public handle;
- an existing email does not reveal account existence;
- successful creation and existing-email attempts both return generic `202`
  without a user object;
- an existing email receives a deduplicated security notice, bounded by a time
  window in addition to endpoint rate limiting.

`POST /api/v1/users/verify_email/resend` accepts an email address and always
returns generic `202`. When the address belongs to an unverified user, it
enqueues a deduplicated verification email. Unknown and verified addresses have
the same response shape and status.

The frontend registration flow displays a generic "check your email" result
instead of consuming a returned user object.

## Demo Opening Balance Limits

Demo opening balances remain supported. New `ACCOUNT_OPENING_LIMITS` config is
a JSON object mapping currency codes to maximum balances in minor units. Missing
currencies have a cap of zero, which permits account creation but no opening
funds.

Checked-in demo profiles use:

```json
{"USD":100000,"EUR":100000,"VND":25000000}
```

`POST /api/v1/accounts` returns `422` when `balance` exceeds the configured cap.
The existing non-negative validation remains. A public
`GET /api/v1/account-opening-limits` endpoint returns the effective map so the
SPA can show and validate the same policy. Server validation remains
authoritative.

## Error Semantics

- `401 Unauthorized`: invalid, expired, revoked, rotated, or wrong-purpose
  token.
- `403 Forbidden`: valid login credentials for an unverified user.
- `409 Conflict`: username already exists or an idempotency key is reused with
  different transfer parameters.
- `422 Unprocessable Entity`: opening balance exceeds its currency cap.
- `202 Accepted`: registration accepted generically or verification resend
  accepted generically.
- `204 No Content`: logout completed or was already effectively complete.

Error bodies retain the service's existing client-safe `{"error":"..."}`
shape. Internal database and token details remain hidden.

## Security Properties

- A refresh token cannot authorize account or transfer routes.
- JavaScript cannot read refresh credentials.
- Server logout prevents later renewal with the logged-out session.
- A successful renewal invalidates the token used for that renewal.
- Idempotency replay cannot cross source-account ownership boundaries.
- Unverified identities cannot create accounts or transfer funds.
- Registration and resend do not disclose email membership through HTTP status
  or response shape.
- Opening demo funds are bounded by server-side currency policy.

## Test Strategy

Implementation follows test-driven, risk-first slices:

1. Add a failing API test proving a refresh token currently works as bearer,
   then add typed-token unit and middleware tests until cross-use fails.
2. Add an integration test reproducing cross-user idempotency replay and a
   mismatched-payload test, then change query scope and uniqueness.
3. Add handler and integration tests for HttpOnly cookie login, body-free
   renewal, one-time rotation, concurrent renewal, logout revocation, cookie
   expiry, and omission of refresh secrets from JSON.
4. Add tests for unverified login denial, no session creation, generic
   registration responses, username conflict, resend behavior, and job
   deduplication.
5. Add config, handler, and frontend tests for opening caps at below, equal, and
   above boundary values in each supported currency.

Repository validation uses only checked-in `mise` tasks:

- `mise run test:unit`
- `mise run test:integration`
- `mise run frontend:test`
- `mise run frontend:check`
- `mise run frontend:lint`
- `mise run golangci-lint`
- `mise run govulncheck`

## Rollout

The idempotency migration replaces the old global unique constraint with a
composite one. Existing rows already satisfy the new constraint because global
uniqueness is stronger.

Legacy localStorage refresh tokens are intentionally discarded. Existing
database sessions remain harmless but unused and expire naturally. Users log in
again to receive the cookie-based session. No dual-read compatibility path is
provided because retaining browser-readable refresh tokens would preserve the
vulnerability.
