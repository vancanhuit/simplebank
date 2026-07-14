# Task 1: Add dependencies and sqlc/goose tooling config

**Files:**
- Modify: `go.mod`
- Create: `sqlc.yaml`
- Modify: `mise.toml`

**Goal:** Add all Go dependencies for the project and set up sqlc config + a mise generate task.

## Steps

### Step 1: Add Go dependencies
Run:
```bash
go get github.com/labstack/echo-jwt/v5@latest
go get github.com/golang-jwt/jwt/v5@latest
go get github.com/jackc/pgx/v5@latest
go get github.com/pressly/goose/v3@latest
go get github.com/riverqueue/river@latest
go get github.com/riverqueue/river/riverdriver/riverpgxv5@latest
go get github.com/riverqueue/river/rivermigrate@latest
go get github.com/wneessen/go-mail@latest
go get github.com/urfave/cli/v3@latest
go get golang.org/x/crypto/bcrypt@latest
go get golang.org/x/time/rate@latest
go get github.com/google/uuid@latest
```
Expected: `go.mod` lists these modules; no errors.

### Step 2: Create `sqlc.yaml`
```yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "internal/db/migrations"
    queries: "internal/db/query"
    gen:
      go:
        package: "db"
        out: "internal/db/sqlc"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_interface: true
        emit_empty_slices: true
        overrides:
          - db_type: "uuid"
            go_type: "github.com/google/uuid.UUID"
          - db_type: "timestamptz"
            go_type: "time.Time"
```

### Step 3: Add sqlc generate task to `mise.toml`
Append this task block (keep existing tasks intact):
```toml
[tasks."sqlc:generate"]
description = "Generate Go code from SQL with sqlc"
run = [
  'go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest',
  'sqlc generate',
]
```

### Step 4: Commit
```bash
git add go.mod go.sum sqlc.yaml mise.toml
git commit -m "chore: add pgx sqlc goose river go-mail cli deps and sqlc config"
```

## Global Constraints (must hold)
- Go module path: `github.com/vancanhuit/simplebank`; Go `1.26.5`.
- Do NOT run `sqlc generate` yet (no schema/queries exist until later tasks) — only install the tool config. `go build ./...` at this stage only needs to succeed for existing packages; adding deps to go.mod that are not yet imported is fine (they will show as require entries, possibly marked indirect until used).
- Use conventional commit messages.

## Report contract
Write your full report to `.superpowers/sdd/task-1-report.md`. Return only: status (DONE / DONE_WITH_CONCERNS / NEEDS_CONTEXT / BLOCKED), the commit hash(es), a one-line summary, and any concerns.
