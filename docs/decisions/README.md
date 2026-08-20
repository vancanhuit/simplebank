# Architecture Decision Records

This directory records significant, hard-to-reverse technical decisions for
SimpleBank. Each ADR captures the context, the decision, the alternatives that
were rejected, and the consequences — the *why* that the code itself cannot show.

Do not delete superseded ADRs; write a new one that references and supersedes
the old. See [ADR-0000](0000-record-architecture-decisions.md) for the process.

| ADR | Title | Status |
|-----|-------|--------|
| [0000](0000-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0001](0001-wide-sqlc-backed-store-interface.md) | Wide sqlc-backed `Store` interface | Accepted |
| [0002](0002-hash-refresh-tokens-at-rest.md) | Hash refresh tokens at rest | Accepted |
| [0003](0003-server-owns-routing-with-injected-readiness.md) | Server owns HTTP routing with an injected readiness probe | Partially superseded by [0006](0006-run-worker-with-http-server.md) |
| [0004](0004-split-util-into-domain-packages.md) | Split `util` into domain packages, separating crypto from non-crypto randomness | Accepted |
| [0005](0005-transfer-safety-idempotency-and-limits.md) | Make transfers idempotent and enforce limits at safe boundaries | Accepted |
| [0006](0006-run-worker-with-http-server.md) | Run the River worker with the HTTP server | Accepted |
| [0007](0007-use-postgresql-for-revocable-access-and-login-throttling.md) | Use PostgreSQL for revocable access and login throttling | Accepted |
