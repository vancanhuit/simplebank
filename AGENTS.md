# SimpleBank Agent Notes

## Sources and tools

- Use the pinned `mise` toolchain and `mise run ...` tasks from `mise.toml`; `auto_install` is disabled, so run `mise install` on a fresh checkout.
- Treat current code, `README.md`, `frontend/README.md`, and accepted ADRs in `docs/decisions/` as authoritative. `docs/superpowers/{specs,plans}/` is historical and may be stale.
- Go-wide build, lint, vulnerability, and unit-test tasks build the SPA first because `frontend/embed.go` embeds `frontend/dist`; do not remove that dependency or expect a clean direct Go build of `cmd/app` to work without the assets.

## Architecture boundaries

- `cmd/app` is the composition root: configuration, domain and River migrations, shared dependencies, and lifecycle ordering belong there. Preserve startup as migrations -> River -> HTTP and shutdown as HTTP -> River -> database pool.
- `internal/api` owns Echo routes, middleware, request validation, authorization, and HTTP error mapping. It depends on the wide `internal/db.Store`; do not add per-handler repository wrappers or narrow store interfaces (ADR-0001).
- Edit SQL in `internal/db/query/*.sql` and schema in `internal/db/migrations/*.sql`. Put transaction orchestration in handwritten `internal/db/*_tx.go`; never edit generated `internal/db/sqlc/`.
- Money is `int64` minor units, never floating point. Currency rules belong in `internal/currency` and configured backend policy; the SPA reads policy endpoints rather than duplicating limits.
- Transfer changes must preserve source-first authorization, source-scoped idempotency, deterministic account-lock ordering, validation on locked rows, the rolling daily limit, and guarded balance updates (ADR-0005).
- Cryptographic tokens belong in `internal/secret`; `internal/random` is only for non-cryptographic fixtures/display data.
- The Svelte 5 SPA shares the API origin in production. API and health routes must remain ahead of the SPA fallback; unknown `/api` paths must stay JSON 404s.
- Frontend account state is session-scoped; preserve reset generation checks so responses started before logout cannot repopulate the next session.

## Generated code and migrations

- After any sqlc query, migration, or `sqlc.yaml` change, run `mise run sqlc:generate` and commit source plus regenerated output together. The pre-commit hook rejects unstaged inputs and stale `internal/db/sqlc/` output.
- Domain and River migrations run automatically at service startup. Integration-test setup also applies embedded domain migrations; do not add a separate manual migration prerequisite.

## Focused iteration

- Go unit/package test: `go test -race ./internal/api -run '^TestName$'`. Build `frontend/dist` first with `mise run frontend:build` when the tested package graph reaches `frontend`/`cmd/app` on a clean tree.
- Frontend test file: run `mise run frontend:install` once, then `mise run frontend:test -- src/lib/money.test.ts`; use the package's `test:watch` script directly from `frontend/` only for interactive iteration.
- Focused integration test: start PostgreSQL with `mise run compose:test:up`, run `go test -race -tags=integration ./internal/db -run '^TestName$'`, then always run `mise run compose:test:down`. The full `mise run test:integration` task owns this lifecycle automatically.
- Playwright starts its own Vite server and mocks API responses, so it does not need the Go/Compose stack. Install Chromium once with `mise run frontend:test:e2e:install`; snapshots live beside `frontend/e2e/accessibility.spec.ts`.

## Completion gates

- Backend: `mise run golangci-lint:fmt`, `mise run golangci-lint`, `mise run test:unit`.
- Database queries, migrations, locking, or transactions: also `mise run test:integration`.
- Frontend: `mise run frontend:check`, `mise run frontend:lint`, `mise run frontend:format:check`, `mise run frontend:test`, `mise run frontend:test:e2e`.
- Composition, SPA embedding, CLI, or release-build changes: also `mise run app:build` and exercise the affected runtime path. Run directly with `mise run app -- serve` and required `DB_SOURCE`, `JWT_SECRET` (at least 32 characters), and `SMTP_FROM`.
- Security/dependency-sensitive changes: also `mise run govulncheck`.
- Commits use Conventional Commits. `mise run hooks:install` installs the formatting/sqlc checks enforced by `.githooks/pre-commit`.
