# ADR-0002: Hash refresh tokens at rest

## Status
Accepted

## Date
2026-07-15

## Context
Login issues a long-lived refresh token (a signed JWT) and persists a session
row in `sessions` so the token can be revoked and rotated. Originally the raw
refresh token was stored in `sessions.refresh_token`. A read-only leak of the
sessions table (backup exposure, SQL injection elsewhere, over-broad DB access)
would therefore hand an attacker usable refresh tokens for every active session.

The refresh token is high-value: it mints new access tokens without
re-authentication. Unlike a password, we never need to display or reverse it —
we only need to check that a presented token matches the stored one.

## Decision
Store a SHA-256 hash (hex-encoded) of the refresh token in
`sessions.refresh_token`, not the raw token. The raw token is returned to the
client once at login; the server keeps only the hash. On renew, hash the
presented token and compare it against the stored hash using
`subtle.ConstantTimeCompare`. See `hashRefreshToken` in `internal/api/user.go`.

## Alternatives Considered

### Store the raw token (previous behavior)
- Pros: trivial; direct equality check.
- Cons: a database leak yields immediately usable tokens for every session.
- Rejected: unacceptable blast radius for a read-only leak.

### bcrypt/argon2 instead of SHA-256
- Pros: slow hashing resists brute force.
- Cons: the input is a 256-bit-entropy signed JWT, not a low-entropy password;
  brute force is already infeasible. A slow hash on every renew adds latency
  for no meaningful gain.
- Rejected: SHA-256 is sufficient given the token's entropy; keep renew fast.

### Encrypt the token at rest
- Pros: reversible if we ever needed the raw value.
- Cons: introduces key management, and we never need to reverse it — only
  compare. Encryption is the wrong tool for a compare-only secret.
- Rejected: unnecessary complexity.

## Consequences
- A read-only leak of `sessions` no longer yields usable refresh tokens.
- The column type is unchanged (still stores a string); no migration needed.
- Sessions created before this change hold raw tokens and will no longer match
  the hash of a presented token, so those sessions are effectively invalidated.
  Affected users simply log in again. This one-time cost was accepted.
- Renew comparison is constant-time to avoid leaking match information via
  timing, consistent with the login enumeration mitigation.
