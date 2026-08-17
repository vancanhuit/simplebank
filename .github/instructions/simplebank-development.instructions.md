---
name: "SimpleBank Development"
description: "Use for all SimpleBank code, feature, bug-fix, database, frontend, test, and documentation work. Covers architecture boundaries, coding conventions, sqlc generation, and required verification commands."
applyTo: "**"
---
# SimpleBank Development

## Sources of Truth

- Treat `README.md` and `frontend/README.md` as the current guides. Treat accepted records in `docs/decisions/` as authoritative for durable architecture decisions.
- Treat `docs/superpowers/specs/` and `docs/superpowers/plans/` as historical context; current code, guides, and ADRs may supersede them.
- Use the pinned tools and `mise run ...` tasks from `mise.toml`. Do not substitute ad hoc commands when a project task exists.

## Architecture and Conventions

- Keep `cmd/app` as the composition root. It owns configuration, migrations, shared dependencies, River startup, HTTP startup, and ordered shutdown.
- Keep HTTP routes, handlers, middleware, validation, and error mapping in `internal/api`. The server owns route registration and receives readiness as an injected probe.
- Keep database access behind the wide `internal/db.Store` seam described by ADR-0001. Do not introduce per-handler store interfaces or pass-through repository wrappers without a superseding architecture decision.
- Keep transaction orchestration in hand-written `internal/db/*_tx.go` methods. Preserve transfer idempotency, deterministic account lock ordering, locked-row validation, daily-limit enforcement, and guarded balance updates from ADR-0005.
- Run River in the HTTP process. Preserve startup order (River before HTTP) and shutdown order (HTTP, River, database pool) from ADR-0006.
- Keep domain helpers in focused packages. Cryptographic token generation belongs in `internal/secret`; `internal/random` is only for non-cryptographic fixture or display data.
- Represent money as integer minor units. Keep currency rules in `internal/currency` and configured policy; do not use floating-point amounts or duplicate backend policy in the frontend.
- Keep the Svelte 5 SPA in `frontend`. Production assets are embedded in the Go binary and share the API origin. Follow nearby Svelte/TypeScript patterns and preserve keyboard, screen-reader, responsive, and axe-tested behavior.
- Follow existing package-local test patterns. Add or update focused tests for every behavior change and regression fix.

## SQLC and Migrations

- Edit query sources in `internal/db/query/*.sql` and schema migrations in `internal/db/migrations/`.
- Never hand-edit `internal/db/sqlc/`; it is generated from `sqlc.yaml`.
- After changing SQL queries or schema used by sqlc, run `mise run sqlc:generate` and review the generated diff. Keep generated changes in the same change as their source SQL.
- Add transaction behavior around generated queries in `internal/db`, not in generated files. Run integration tests for query, migration, locking, or transaction changes.

## Verification

- Start with the narrowest relevant test while iterating, then run every applicable project gate before reporting completion.
- Go/backend changes: `mise run golangci-lint:fmt`, `mise run golangci-lint`, and `mise run test:unit`.
- PostgreSQL query, migration, transaction, locking, or integration behavior: also run `mise run test:integration`; the task owns the test Compose lifecycle.
- Frontend changes: run `mise run frontend:check`, `mise run frontend:lint`, `mise run frontend:format:check`, `mise run frontend:test`, and `mise run frontend:test:e2e`.
- Cross-stack, SPA embedding, CLI wiring, or release-build changes: run `mise run app:build` and exercise the affected runtime path.
- Security- or dependency-sensitive changes: run `mise run govulncheck`.
- If a required gate cannot run, report the exact command, blocker, and unverified risk. Never claim completion from code inspection alone.

## Documentation

- Update current guides when setup, commands, configuration, API behavior, test workflows, or repository structure changes.
- Add an ADR for a significant decision that is expensive to reverse. Supersede prior ADRs explicitly; do not delete or rewrite accepted history.
