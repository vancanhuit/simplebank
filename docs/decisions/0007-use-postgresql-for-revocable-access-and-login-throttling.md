# ADR-0007: Use PostgreSQL for revocable access and login throttling

## Status
Accepted

## Date
2026-08-20

## Context
SimpleBank already persists refresh-token sessions in PostgreSQL so logout and
rotation can revoke long-lived credentials. Earlier hardening work also added
shared PostgreSQL-backed login-throttle state keyed by account and client IP so
all API replicas enforce the same cooldowns.

Access JWTs were still effectively stateless: once minted, a protected route
only verified signature, expiry, and token type. Logging out blocked the
refresh-session row, but any already-issued access token stayed usable until its
TTL elapsed. That weakened the value of the revocable session store and made the
new throttle/session controls asymmetric across refresh and access credentials.

We need one revocation model that works across replicas, survives restarts, and
keeps the existing `internal/db.Store` ownership and API flow.

## Decision
Use PostgreSQL as the shared control plane for both revocable sessions and login
throttling.

- Every protected request verifies the access JWT signature locally, then reads
  the matching `sessions` row by ID and rejects the request when the row is
  missing, blocked, expired, or belongs to a different username.
- Access JWTs, refresh JWTs, and the persisted `sessions.id` now share one
  stable UUID. Login creates a new UUIDv7 when opening a session; refresh-token
  renewal reuses the existing session ID while rotating token material in place.
- Newly issued JWTs carry a signed `session_bound=true` claim. Protected-route
  middleware always validates access tokens with that claim against PostgreSQL.
  Access tokens whose claim is missing or false are rejected unless the
  explicit `ALLOW_LEGACY_ACCESS_TOKENS` compatibility flag is enabled.
- PostgreSQL remains the source of truth for login-throttle state keyed by
  account and client IP. Login atomically locks and reserves both counters
  before user lookup or bcrypt. The threshold reservation is admitted and sets
  cooldown for later requests; a request that finds either counter actively
  blocked rolls back without incrementing either counter.

### Rolling deployment

The legacy-token flag defaults to false and exists only to bridge replicas that
still mint access JWTs without `session_bound`:

1. Enable `ALLOW_LEGACY_ACCESS_TOKENS` on new replicas while old replicas remain.
2. Remove every old replica.
3. Wait one full configured `ACCESS_TTL` after the last old replica is removed.
4. Disable `ALLOW_LEGACY_ACCESS_TOKENS`.

Atomic deployments can keep the flag false. While compatibility is enabled,
legacy access tokens remain valid by signature and expiry only; they cannot be
revoked through the session table. Session-bound tokens never bypass
PostgreSQL validation, regardless of the flag.

## Alternatives Considered

### Stateless access tokens with short TTL only
- Pros: no database read on protected requests.
- Cons: logout cannot immediately revoke an already-issued access token; every
  compromise window lasts until expiry.
- Rejected: immediate server-side revocation is required for session-backed
  logout and rotation semantics.

### Redis for revocation and throttling state
- Pros: low-latency reads, purpose-built ephemeral state store.
- Cons: adds another distributed dependency, deployment surface, failure mode,
  and consistency story for data already modeled in PostgreSQL.
- Rejected: PostgreSQL already stores sessions durably and now stores throttle
  state; duplicating that control plane is unnecessary complexity.

### Proxy-only rate limits and revocation
- Pros: keeps application handlers simpler.
- Cons: the proxy cannot validate application session ownership, refresh-token
  rotation state, or per-account cooldown rules tied to authenticated users.
- Rejected: edge limits can complement the app, but they cannot replace
  session-aware revocation and account/IP-aware throttling.

## Consequences
- Each protected request adds one indexed PostgreSQL session read after JWT
  verification.
- Logout now revokes the paired access token immediately because both JWT types
  and the session row share the same stable ID.
- Session revocation and login-throttle decisions are shared across replicas and
  survive process restarts.
- Rolling upgrades have an explicit, temporary compatibility escape hatch for
  pre-claim access JWTs. Leaving it enabled extends the unrevocable legacy-token
  window, so operators must disable it after one access-token TTL.
- API handlers keep their existing `internal/db.Store` seam; session validation
  and atomic throttle admission stay centralized in backend store methods
  rather than spreading replica-local caches through handlers or middleware.
