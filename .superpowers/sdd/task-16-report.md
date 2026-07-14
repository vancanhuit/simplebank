# Task 16 Report: Update compose and Dockerfile for full app

**Status:** DONE

## Changes
- `compose.yaml`: Expanded `app-dev` service with `command: ["serve"]`, `environment` (HTTP_ADDR, DB_SOURCE, JWT_SECRET, SMTP_HOST, SMTP_PORT, SMTP_FROM), and `depends_on: postgres-dev (service_healthy)`. All other services/anchors/networks unchanged.
- `Dockerfile`: Added `COPY internal internal` immediately after `COPY cmd cmd` so the build includes the new `internal/...` packages.

## Step 3: Docker build
`docker compose --profile dev build app-dev` — **SUCCESS**.

Key build steps:
- `[builder 11/13] COPY cmd cmd` — DONE
- `[builder 12/13] COPY internal internal` — DONE
- `[builder 13/13] ... mise run app:build` → `go build -ldflags="-s -w" -o dist/simplebank cmd/app/main.go` — DONE (10.9s)
- Image `simplebank-app-dev` built.

## Step 4: Startup migrations verification
Started `postgres-dev` (reached Healthy), then ran `go run ./cmd/app serve` against the dev DB.

Startup log lines:
```
{"time":"2026-07-14T21:28:53.929677292+07:00","level":"INFO","msg":"domain migrations applied"}
{"time":"2026-07-14T21:28:53.980583462+07:00","level":"INFO","msg":"river migrations applied"}
{"time":"2026-07-14T21:28:53.98089892+07:00","level":"INFO","msg":"Echo (v5.3.0). High performance, minimalist Go web framework https://echo.labstack.com","version":"5.3.0"}
{"time":"2026-07-14T21:28:53.980908639+07:00","level":"INFO","msg":"http(s) server started","address":"[::]:8080"}
```

Both migration lines present ("domain migrations applied", "river migrations applied") and the HTTP server started listening on `:8080`. No mailer dial error at startup (SMTP_HOST=localhost unreachable — as expected, `mail.NewSMTPMailer` does not dial). Server stopped; environment torn down with `docker compose --profile dev down -v`.

## Concerns
None.
