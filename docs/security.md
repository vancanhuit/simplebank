# Security

This guide describes the security controls implemented by SimpleBank and the
responsibilities that remain with a production deployment. It is not a claim of
formal certification or a substitute for a deployment-specific threat model.

## Transport and Proxy Trust

- Serve the application over HTTPS, either directly with `TLS_CERT_FILE` and
  `TLS_KEY_FILE` or behind a TLS-terminating reverse proxy.
- Keep `SESSION_COOKIE_SECURE=true` in production. Configuration validation
  rejects an HTTPS `PUBLIC_BASE_URL` combined with an insecure session cookie.
- Set `PUBLIC_BASE_URL` to the exact external origin. It is used for email links
  and as the expected origin for cookie-authenticated browser requests.
- Set `TRUSTED_PROXIES` only to CIDRs or IPs of proxies that can connect directly
  to the application. With no trusted proxies, the application strips
  `X-Forwarded-For`, `X-Forwarded-Proto`, and `X-Forwarded-Host` and uses the
  socket peer as the client address.
- Do not expose PostgreSQL, Mailpit, or pgAdmin publicly. The development Compose
  profiles bind published ports to `127.0.0.1`; they are not production
  configurations.

The HTTP server limits request bodies to 1 MiB, applies 30-second request and
handler deadlines, and configures bounded header, response, and idle timeouts.
Responses receive a restrictive Content Security Policy, `X-Frame-Options:
DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, and a
one-year HSTS policy. API responses also use `Cache-Control: no-store`.

## Authentication and Credentials

- Registration requires at least 15 characters and accepts at most 72 password
  bytes, the bcrypt input limit. Passwords are stored as bcrypt hashes.
- Registration and verification resend return generic accepted responses so
  callers cannot determine whether a username or email exists. Unknown-user
  login attempts still perform a bcrypt comparison to reduce timing disclosure.
- Access JWTs are accepted only as bearer credentials and are held in SPA memory.
  They expire after `ACCESS_TTL`, which defaults to 15 minutes.
- Refresh JWTs are placed in the host-only `simplebank_refresh` cookie with
  `HttpOnly`, `SameSite=Strict`, path `/api/v1`, and `Secure` enabled by default.
  Their SHA-256 digests, not reusable raw tokens, are stored in PostgreSQL.
- Refresh sessions rotate atomically. Logout blocks the refresh session, expires
  the cookie even when revocation fails, and the SPA always clears its local
  access state.
- Renew and logout validate `Origin` and Fetch Metadata when browsers provide
  them. Requests from another origin are rejected before the refresh cookie is
  consumed; non-browser API clients may omit those headers.
- Email verification codes are generated cryptographically and stored only as
  SHA-256 digests. Request logging records the URL path rather than the query
  string, and the SPA removes verification credentials from browser history
  after reading them.

Migration `00005_hash_email_verification_codes.sql` invalidates verification
links created before the migration because existing plaintext codes cannot be
safely converted to digests. Affected users must request a new verification
email. Rolling the migration back does not recover those links.

## Authorization and Data Exposure

Handlers enforce ownership for account reads and transfer history. Transfers
authorize the source account before looking up the destination, scope
idempotency to the source, lock accounts in deterministic order, validate the
locked rows, enforce configured limits, and guard balance updates against
overdrafts. See
[ADR-0005](decisions/0005-transfer-safety-idempotency-and-limits.md).

A successful transfer response includes the transfer and the authenticated
caller's updated source account. It deliberately omits the destination account
snapshot so a sender cannot observe the recipient's balance or account state.

## Abuse and Supply-Chain Controls

Registration and login use a stricter per-client token bucket; renew, logout,
and email verification endpoints are also rate limited. These limiters use the
trusted client address described above and keep separate buckets per endpoint,
so activity on one flow does not throttle another. Limited responses include
`Retry-After` and rate-limit metadata for clients to present useful feedback.

CI runs Go vulnerability analysis and frontend dependency auditing in addition
to lint, unit, integration, browser, application, and container build checks.
GitHub Actions are pinned to commit SHAs, build tools are version-pinned, and the
Docker build verifies the downloaded mise binary checksum. The runtime image is
distroless and runs as a non-root user.

Run the dependency checks locally with:

```sh
mise run govulncheck
mise run frontend:audit
```

## Production Checklist

- Generate a unique, high-entropy `JWT_SECRET` of at least 32 characters and
  store it with database and SMTP credentials in a secrets manager.
- Require TLS for public HTTP, PostgreSQL, and SMTP connections; leave insecure
  modes confined to isolated local development.
- Set `PUBLIC_BASE_URL`, keep secure cookies enabled, and configure the smallest
  accurate `TRUSTED_PROXIES` ranges.
- Restrict database and administrative service network access. Do not reuse the
  throwaway credentials from `compose.yaml`.
- Set transfer and account-opening limits as business risk policy rather than
  copying the demonstration values.
- Monitor authentication throttling, authorization failures, migration errors,
  River job failures, and abnormal transfer activity without logging secrets or
  raw query strings.

## Known Limitations

- Rate-limit state is in process memory. Each replica enforces its own limit, so
  multi-replica deployments need a shared gateway or edge rate limiter.
- Logout revokes refresh capability but cannot revoke an access JWT already
  issued to another client. That token remains valid until `ACCESS_TTL` expires.
- Startup migrations use the same `DB_SOURCE` credential as runtime queries.
  Deployments requiring strict least privilege need a separate migration phase
  and a runtime role without schema-change permissions.
- Releases publish binary checksums, but container images are not yet signed and
  the release workflow does not produce provenance attestations or an SBOM.
- Multi-factor authentication, account recovery, and password reset are not
  implemented.
