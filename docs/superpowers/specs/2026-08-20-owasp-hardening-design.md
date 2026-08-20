# OWASP Hardening Design

## Status

Approved in chat on 2026-08-20.

## Context

The OWASP audit identified eight gaps:

- transfer responses disclose recipient account ownership and live balances;
- logout does not immediately revoke already-issued access tokens;
- registration accepts six-character passwords without MFA;
- username collisions expose account existence;
- authentication throttling is process-local and permissive;
- HTTP responses omit explicit CSP, HSTS, and referrer policy;
- the container build executes a mutable remote installer;
- GitHub workflows execute third-party actions through mutable major tags.

The application already keeps refresh sessions in PostgreSQL, routes database
access through `internal/db.Store`, serves the SPA and API from one origin, and
uses pinned tools through `mise.toml` and `mise.lock`. The hardening should reuse
those boundaries instead of adding Redis, a separate identity provider, or a
second proxy-specific security policy.

## Goals

- Return only caller-authorized transfer data.
- Revoke both refresh and access credentials immediately on logout.
- Require passwords of 15 to 72 bytes while MFA is absent.
- Make registration responses indistinguishable for existing usernames and
  email addresses.
- Share authentication throttling across horizontally scaled instances.
- Apply explicit browser security headers to API and SPA responses.
- Make build-time installers and CI actions immutable and reviewable.
- Preserve existing same-origin SPA behavior, refresh rotation, and the wide
  sqlc-backed store seam.

## Non-Goals

- Multi-factor authentication, password recovery, or a breached-password API.
- User-facing session management or revoking other devices on logout.
- Redis or reverse-proxy-specific rate limiting.
- Changing transfer persistence or idempotency semantics.
- Introducing cross-origin API access.

## Session-Bound Access Tokens

Login creates one stable session UUID and uses it as the ID claim in both the
access and refresh JWTs. The session row remains the source of truth for
revocation, blocked state, username, and expiry.

Protected-route middleware verifies the access JWT, loads its session through
the existing `Store`, and rejects the request unless:

- the session exists;
- the session is not blocked;
- the session username matches the JWT username;
- the session expiry is still in the future.

This adds one indexed PostgreSQL read to each protected request. That cost is
accepted because immediate logout revocation is required and PostgreSQL is
already the shared consistency boundary. Logout continues blocking the stable
session row, so subsequent access-token requests fail immediately. Refresh
rotation preserves the same session ID.

Missing, expired, blocked, or mismatched sessions return the existing generic
`401` response. Database failures remain server errors rather than being
silently treated as authorization failures.

This decision extends the session lifecycle defined by the 2026-08-11
authentication hardening design, whose non-goal excluded access-token
revocation beyond short expiry.

## Shared Login Throttling

Authentication attempt state is stored in PostgreSQL and accessed through sqlc
queries exposed by the existing `Store` seam. No handler-local repository
interface is introduced.

Each login attempt updates two normalized keys:

- account key derived from the submitted username;
- client key derived from Echo's trusted `RealIP`.

Counters use bounded windows and progressive cooldown. A request is rejected
when either key is blocked. Unknown and known usernames follow the same counter
path and return the same public error. Successful authentication clears the
account key; client counters naturally expire so one successful account cannot
erase abuse against other accounts from the same source.

Old rows receive an expiry and are deleted opportunistically or by a focused
cleanup query, preventing unbounded growth. The existing lightweight in-memory
limiter may remain for registration, verification, renew, and logout abuse, but
login security no longer depends on process-local state.

`429 Too Many Requests` uses a generic message and `Retry-After` derived from
the active cooldown.

## Registration and Password Policy

Registration requires passwords between 15 and 72 bytes. Fifteen follows OWASP
guidance for password-only authentication; 72 preserves bcrypt's input limit.
The frontend hint and input metadata match the server rule, but server
validation remains authoritative.

New-user creation still hashes and stores the password and queues verification
mail. Existing username and existing email paths both return the same
`202 Accepted` body as successful creation. Existing email behavior keeps the
deduplicated registration notice. Existing username behavior performs no
account-specific response.

This intentionally supersedes the earlier decision to expose username
collisions with `409 Conflict`. Usernames may be public after registration, but
registration is not an authenticated directory and must not confirm account
existence.

## Transfer Response Boundary

`TransferTxResult` remains an internal transaction result because database and
integration code need both account rows and entry rows. The HTTP handler maps it
to a dedicated public response containing:

- the transfer;
- the caller-owned source account.

The recipient account, recipient owner, recipient balance, and internal ledger
entries are omitted. Idempotent replay returns the same public shape, so it
cannot be used as a recipient-balance polling endpoint.

## Browser Security Headers

Echo uses explicit secure middleware configuration rather than defaults:

- `Content-Security-Policy`: same-origin defaults, no objects, no framing, and
  only resources required by the generated SPA;
- `Strict-Transport-Security`: one year, enabled for HTTPS responses;
- `Referrer-Policy: no-referrer`;
- `X-Content-Type-Options: nosniff`;
- `X-Frame-Options: DENY`.

The CSP is validated against the built SPA before adoption. Header policy lives
in the application so direct TLS and reverse-proxy deployments behave
consistently.

## Supply-Chain Integrity

The Docker build no longer pipes `https://mise.run` into a shell. It installs a
specific mise release artifact and verifies its published SHA-256 checksum
before execution. Tool installation after that point continues using
`mise.toml` and `mise.lock`, whose tool artifacts already carry checksums.

Every third-party GitHub Action reference in workflows and composite actions is
pinned to a full commit SHA. A version comment remains beside each SHA for
readability. Dependabot's GitHub Actions ecosystem continues opening update PRs,
making SHA movement explicit and reviewable.

Base images should also remain digest-pinned where practical, with Dependabot
updating those references through review.

## Error Semantics

- `202 Accepted`: registration accepted, including username/email collisions.
- `400 Bad Request`: password or payload validation failure.
- `401 Unauthorized`: invalid access token or invalid/revoked session.
- `429 Too Many Requests`: shared login throttle active.
- Existing transfer errors remain unchanged.

No response reveals whether a submitted username exists.

## Testing

Focused tests cover:

- 14-byte rejection and 15-byte acceptance;
- generic `202` for username and email collisions;
- account and client throttle counters, cooldown expiry, successful-login
  account reset, and cross-instance behavior through PostgreSQL;
- access-token rejection after logout blocks the shared session;
- missing, expired, blocked, and username-mismatched sessions;
- transfer responses omit `to_account`, recipient ownership/balance, and entry
  objects on new and replayed transfers;
- exact CSP, HSTS, referrer, frame, and content-type headers;
- Docker installer version and checksum verification;
- full-SHA action references in workflows and composite actions.

Required project gates:

- `mise run sqlc:generate` after query or schema changes;
- `mise run golangci-lint:fmt`;
- `mise run golangci-lint`;
- `mise run test:unit`;
- `mise run test:integration`;
- `mise run frontend:check`;
- `mise run frontend:lint`;
- `mise run frontend:format:check`;
- `mise run frontend:test`;
- `mise run frontend:test:e2e`;
- `mise run app:build`;
- `mise run govulncheck`.

## Consequences

- Protected requests gain one indexed database read.
- Login throttling becomes consistent across replicas but adds schema, query,
  cleanup, and integration-test surface.
- Registration clients can no longer distinguish unavailable usernames before
  attempting signup.
- Transfer API responses become intentionally narrower.
- CSP changes require coordinated updates when new frontend resource origins or
  inline execution patterns are introduced.
- CI action updates become SHA diffs instead of simple major-tag changes.
