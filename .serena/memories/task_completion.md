# Task Completion

- Go/backend change: run `mise run golangci-lint:fmt`, `mise run golangci-lint`, and `mise run test:unit`.
- PostgreSQL transaction, migration, locking, query, or integration behavior: also run `mise run test:integration` (task manages the test Compose lifecycle).
- SQL query/schema change: run `mise run sqlc:generate`; review generated diff; keep source and generated changes together; run relevant unit/integration gates.
- Frontend change: run `mise run frontend:check`, `mise run frontend:lint`, `mise run frontend:format:check`, `mise run frontend:test`, and `mise run frontend:test:e2e`.
- Cross-stack, SPA embedding, CLI wiring, or release-build change: run `mise run app:build` and exercise the affected runtime path.
- Security/dependency-sensitive change: run `mise run govulncheck`.
- Update `README.md`, `frontend/README.md`, or ADRs when behavior, commands, configuration, API contracts, test workflows, or repository structure change.
- Do not report completion without fresh outputs from task-relevant gates; report exact blockers and unverified risks when a gate cannot run.