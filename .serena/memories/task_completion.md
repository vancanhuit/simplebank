# Task Completion

- Go/backend change: run `mise run golangci-lint` and `mise run test:unit`.
- Formatting touched: run `mise run golangci-lint:fmt`, then confirm lint/tests.
- PostgreSQL transaction, migration, or integration behavior: also run `mise run test:integration` (task manages test Compose lifecycle).
- SQL query/schema change: run `mise run sqlc:generate`; verify generated diff; run relevant unit/integration gates.
- Frontend change: run `mise run frontend:check`, `mise run frontend:lint`, `mise run frontend:format:check`, `mise run frontend:test`, and `mise run frontend:test:e2e`.
- Cross-stack or embedding change: run `mise run app:build`; verify SPA/API behavior from built or Compose-served app.
- Security/dependency-sensitive release work: run `mise run govulncheck`.
- Update `README.md`, `frontend/README.md`, or ADRs when behavior/commands/contracts changed.
- Do not report completion without fresh outputs from task-relevant gates.