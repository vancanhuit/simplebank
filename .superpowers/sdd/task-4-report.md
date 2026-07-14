# Task 4 Report: Domain schema migrations (goose)

## Status
DONE

## Files created
- `internal/db/migrations/00001_init_schema.sql` — goose Up/Down migration creating all domain tables (users, verify_emails, accounts, entries, transfers, sessions) with indexes, verbatim from the brief.
- `internal/db/migrations/embed.go` — package `migrations` exporting `var FS embed.FS` via `//go:embed *.sql`.

## Steps executed
1. Wrote SQL migration exactly as specified in the brief.
2. Wrote embed.go exactly as specified.
3. `go build ./internal/db/migrations/` → success (BUILD_OK).
4. `git add internal/db/migrations/` and committed with conventional message.

## Build summary
`go build ./internal/db/migrations/` succeeded with no errors.

## Commit
- `be6a9cd07e9e0118b0aad130f3738fa0f378e894` — feat: add initial domain schema goose migration

## Constraints honored
- All PKs use `uuid PRIMARY KEY DEFAULT uuidv7()` (Postgres 18 native, no extension).
- Money columns are `bigint`; `transfers.amount` has `CHECK (amount > 0)`.
- No migrations run (no DB connection); files only.
- Conventional commit message used.

## Concerns
None.
