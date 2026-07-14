# Task 16: Update compose and Dockerfile for full app

**Files:**
- Modify: `compose.yaml`
- Modify: `Dockerfile`

(Note: `mise.toml` was already broadened in Task 8; no change needed here unless something is missing.)

## Step 1: Wire the `app-dev` service in `compose.yaml`
Replace the existing `app-dev` service definition with:
```yaml
  app-dev:
    build: .
    command: ["serve"]
    environment:
      HTTP_ADDR: ":8080"
      DB_SOURCE: "postgres://simplebank_dev:simplebank_dev@postgres-dev:5432/simplebank_dev?sslmode=disable"
      JWT_SECRET: "0123456789012345678901234567890123456789"
      SMTP_HOST: "mailpit-dev"
      SMTP_PORT: "1025"
      SMTP_FROM: "no-reply@simplebank.local"
    ports:
      - "8080:8080"
    networks:
      - dev
    depends_on:
      postgres-dev:
        condition: service_healthy
    profiles:
      - dev
```
Keep everything else in compose.yaml unchanged (the postgres/mailpit services, anchors, networks). Ensure YAML indentation matches the file (2 spaces per level under `services:`).

## Step 2: Update `Dockerfile` to copy `internal/`
The Dockerfile currently does `COPY cmd cmd` before building. Add `COPY internal internal` immediately after that line so the build includes the new packages. (The build command `mise run app:build` compiles `cmd/app/main.go`, which now imports `internal/...`.)

## Step 3: Verify Docker build
Run: `docker compose --profile dev build app-dev`
Expected: image builds successfully (the Go build succeeds inside the container, compiling all `internal/...` packages).

## Step 4: Verify migrations run on `serve` startup against the dev DB
Run:
```bash
docker compose --profile dev up -d --wait postgres-dev
DB_SOURCE="postgres://simplebank_dev:simplebank_dev@localhost:5432/simplebank_dev?sslmode=disable" \
  JWT_SECRET="0123456789012345678901234567890123456789" SMTP_FROM="a@b.c" SMTP_HOST="localhost" \
  go run ./cmd/app serve
```
Expected: logs "domain migrations applied" and "river migrations applied", then the server starts listening on :8080. Stop it with Ctrl-C (or send SIGTERM). Then tear down: `docker compose --profile dev down -v`.

If `go run ./cmd/app serve` can't reach a mail server, that's fine — the mailer client is only constructed, not dialed, at startup (dialing happens in the worker when a job runs). Migrations + HTTP server should still come up. If startup fails specifically because `mail.NewSMTPMailer` errors on an unreachable host, note it in your report (it should NOT — NewClient does not dial).

## Step 5: Commit
```bash
git add compose.yaml Dockerfile
git commit -m "chore: wire app service env and docker build for internal"
```

## Global Constraints
- Container-only ops; no k8s.
- No secrets beyond local dev placeholders in compose (these are non-production dev values).
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-16-report.md`, noting the docker build result and whether `serve` startup migrations succeeded (with the log lines). Return only: status, commit hash(es), one-line summary, concerns.
