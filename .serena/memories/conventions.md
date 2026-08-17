# Conventions

- Go source formatted by `golangci-lint fmt`; lint covers `./cmd/... ./internal/...`.
- SQL source lives in `internal/db/query`; migrations in `internal/db/migrations`; sqlc output in `internal/db/sqlc` is generated and must not be hand-edited. Run `mise run sqlc:generate` after SQL/schema changes and keep generated output with its source.
- Install hooks with `mise run hooks:install`. Pre-commit rejects unformatted staged Go and stale or unstaged sqlc output when database sources change.
- Monetary values are integer minor units; currency policy belongs in `internal/currency` and configured transfer/opening limits. Backend policy is authoritative; frontend consumes policy endpoints rather than duplicating limits.
- Keep cryptographic randomness in `internal/secret`; `internal/random` is only non-crypto fixture/display data.
- Frontend uses TypeScript/Svelte. Prettier: 2 spaces, no tabs, semicolons, double quotes, trailing commas, width 100.
- Frontend ESLint uses JS/TypeScript recommended rules plus Svelte recommended rules and project-aware parsing.
- Update current guides for setup, commands, configuration, API, tests, or repository structure. Add/supersede ADRs for expensive-to-reverse decisions; preserve historical specs/plans.
- Commits use Conventional Commits, enforced by cocogitto.
- Prefer existing package boundaries and injected interfaces; do not bypass the store/server ownership decisions recorded in ADRs.