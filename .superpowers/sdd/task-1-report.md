# Task 1 Report: Add dependencies and sqlc/goose tooling config

**Status:** DONE
**Commit:** `da36896dee5effe12bac7b4b7ac74133c4f59031`
**Branch:** `feat/simplebank-implementation`

## Summary
Added all Go dependencies, created `sqlc.yaml`, appended the `sqlc:generate` mise task, and committed. `go build ./...` succeeds.

## Step details

### Step 1 — Go dependencies
Ran all 12 `go get` commands. Resulting `go.mod` require entries (all currently `// indirect` since not yet imported, which is expected):
- github.com/labstack/echo-jwt/v5 v5.0.2
- github.com/golang-jwt/jwt/v5 v5.3.1
- github.com/jackc/pgx/v5 v5.10.0
- github.com/pressly/goose/v3 v3.27.2
- github.com/riverqueue/river v0.40.0
- github.com/riverqueue/river/riverdriver/riverpgxv5 v0.40.0
- (rivermigrate is part of the river module — no separate require entry)
- github.com/wneessen/go-mail v0.8.1
- github.com/urfave/cli/v3 v3.10.1
- golang.org/x/crypto v0.54.0 (bcrypt)
- golang.org/x/time v0.15.0 (rate)
- github.com/google/uuid v1.6.0

Existing deps preserved: `github.com/labstack/echo/v5 v5.3.0` remains the sole direct require.

### Step 2 — sqlc.yaml
Created `sqlc.yaml` verbatim from the brief (version 2, postgresql engine, pgx/v5 sql_package, uuid + timestamptz overrides). Not run — no schema/queries exist yet, per instructions.

### Step 3 — mise.toml
Appended the `[tasks."sqlc:generate"]` block. All existing task blocks and `[settings]`/`[tools]` sections left intact.

### Step 4 — Commit
Committed the 4 files with the exact message: `chore: add pgx sqlc goose river go-mail cli deps and sqlc config`.

## Build/test verification
`go build ./...` → `BUILD_OK` (exit 0). `sqlc generate` intentionally NOT run.

## Concerns
- `github.com/riverqueue/river/rivermigrate` does not appear as its own require entry — it is a package within the `github.com/riverqueue/river` module, so `go get` resolved it under the parent module. This is correct behavior, not a problem.
- All new deps show `// indirect` until imported by later tasks; expected and acceptable per the brief.
