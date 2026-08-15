# Combined API and Worker Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run River background jobs inside the HTTP API process and remove the standalone worker command and services.

**Architecture:** `serve` assembles one dependency graph, starts River synchronously, then runs the blocking HTTP server while River works in its own library-managed goroutines. HTTP shutdown is followed by a bounded graceful River stop and database pool close; a River startup failure prevents HTTP from listening.

**Tech Stack:** Go 1.26, urfave/cli v3, River v0.43, Echo v5, Docker Compose, standard `testing` package

**Spec:** `docs/superpowers/specs/2026-08-15-combined-api-worker-lifecycle-design.md`

## Global Constraints

- Keep `serve`, `healthcheck`, and `version`; remove only `worker`.
- Do not add dependencies or a generic process supervisor.
- Call River `Start` before HTTP startup and use a non-canceling background context.
- Give River `Stop` 10 seconds before closing the database pool.
- Treat `RIVER_MAX_WORKERS` as a per-API-replica limit.
- Leave historical plans and specs unchanged.
- Do not create commits unless the user explicitly requests them.

---

### Task 1: Combine CLI and Process Lifecycles

**Files:**
- Modify: `cmd/app/main.go`
- Create: `cmd/app/main_test.go`

**Interfaces:**
- Consumes: `(*river.Client[pgx.Tx]).Start(context.Context) error`, `(*river.Client[pgx.Tx]).Stop(context.Context) error`, and existing `startServer`.
- Produces: `newCommand() *cli.Command`, local `workerLifecycle` with `Start(context.Context) error` and `Stop(context.Context) error`, and `runServices(context.Context, workerLifecycle, func(context.Context) error) error`.

- [ ] **Step 1: Write failing command and lifecycle tests**

Create `cmd/app/main_test.go`:

```go
package main

import (
	"context"
	"errors"
	"testing"
)

type fakeWorkerLifecycle struct {
	startErr error
	stopErr  error
	started  bool
	stopped  bool
}

func (worker *fakeWorkerLifecycle) Start(context.Context) error {
	worker.started = true
	return worker.startErr
}

func (worker *fakeWorkerLifecycle) Stop(context.Context) error {
	worker.stopped = true
	return worker.stopErr
}

func TestNewCommandExposesOnlySupportedCommands(t *testing.T) {
	want := map[string]bool{
		"serve":       true,
		"healthcheck": true,
		"version":     true,
	}
	commands := newCommand().Commands
	if len(commands) != len(want) {
		t.Fatalf("command count = %d, want %d", len(commands), len(want))
	}
	for _, command := range commands {
		if !want[command.Name] {
			t.Fatalf("unexpected command %q", command.Name)
		}
	}
}

func TestRunServicesStartFailurePreventsHTTP(t *testing.T) {
	startErr := errors.New("start worker")
	worker := &fakeWorkerLifecycle{startErr: startErr}
	serverStarted := false

	err := runServices(t.Context(), worker, func(context.Context) error {
		serverStarted = true
		return nil
	})

	if !errors.Is(err, startErr) {
		t.Fatalf("runServices error = %v, want wrapped %v", err, startErr)
	}
	if serverStarted {
		t.Fatal("HTTP server started after worker startup failure")
	}
	if worker.stopped {
		t.Fatal("worker stopped after unsuccessful start")
	}
}

func TestRunServicesStopsWorkerAndPreservesErrors(t *testing.T) {
	serverErr := errors.New("serve HTTP")
	stopErr := errors.New("stop worker")
	worker := &fakeWorkerLifecycle{stopErr: stopErr}

	err := runServices(t.Context(), worker, func(context.Context) error {
		if !worker.started {
			t.Fatal("HTTP server started before worker")
		}
		return serverErr
	})

	if !worker.stopped {
		t.Fatal("worker was not stopped after HTTP returned")
	}
	if !errors.Is(err, serverErr) {
		t.Fatalf("runServices error = %v, want server error %v", err, serverErr)
	}
	if !errors.Is(err, stopErr) {
		t.Fatalf("runServices error = %v, want stop error %v", err, stopErr)
	}
}
```

- [ ] **Step 2: Run tests and verify the new contracts are absent**

Run: `go test ./cmd/app`

Expected: FAIL because `newCommand` and `runServices` are undefined.

- [ ] **Step 3: Extract command construction and remove the worker command**

Move the existing `cli.Command` literal into:

```go
func newCommand() *cli.Command {
	return &cli.Command{
		Name:    "simplebank",
		Usage:   "SimpleBank cloud-native service",
		Version: version,
		Flags:   config.Flags(),
		Commands: []*cli.Command{
			{Name: "serve", Usage: "Run the HTTP API server and background worker", Action: runServe},
			{Name: "healthcheck", Usage: "Probe the local liveness endpoint (for container HEALTHCHECK)", Action: runHealthcheck},
			{
				Name:  "version",
				Usage: "Print version information",
				Action: func(_ context.Context, _ *cli.Command) error {
					fmt.Printf("version:    %s\ncommit:     %s\nbuild date: %s\n", version, commit, buildDate)
					return nil
				},
			},
		},
	}
}
```

Replace the literal in `main` with `cmd := newCommand()`. Do not register a
`worker` command.

- [ ] **Step 4: Add the minimal lifecycle coordinator**

Add near `runServe`:

```go
type workerLifecycle interface {
	Start(context.Context) error
	Stop(context.Context) error
}

func runServices(
	ctx context.Context,
	worker workerLifecycle,
	serve func(context.Context) error,
) error {
	if err := worker.Start(context.Background()); err != nil {
		return fmt.Errorf("starting worker: %w", err)
	}
	slog.Info("worker started")

	serverErr := serve(ctx)
	slog.Info("worker shutting down", "cause", context.Cause(ctx))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return errors.Join(serverErr, worker.Stop(shutdownCtx))
}
```

This interface belongs in `main`, where it is consumed. Do not move it into the
worker package.

- [ ] **Step 5: Route `runServe` through the combined lifecycle**

After all API and SPA setup succeeds, replace the direct server call with:

```go
	return runServices(ctx, app.riverClient, func(ctx context.Context) error {
		return startServer(ctx, app.cfg, server.Handler())
	})
```

Delete `runWorker`. Update `appDeps` and `buildApp` comments so they describe
one service dependency graph rather than two entrypoints.

- [ ] **Step 6: Format and run focused tests with the race detector**

Run: `gofmt -w cmd/app/main.go cmd/app/main_test.go`

Run: `go test -race ./cmd/app`

Expected: PASS. Logs may include worker start and shutdown messages from the
fake lifecycle test.

- [ ] **Step 7: Review Task 1 diff**

Run: `git diff --check -- cmd/app/main.go cmd/app/main_test.go`

Expected: no output.

---

### Task 2: Collapse Compose Services

**Files:**
- Modify: `compose.yaml`
- Modify: `mise.toml`

**Interfaces:**
- Consumes: combined `simplebank serve` process from Task 1.
- Produces: one app service per development profile, each running HTTP and River.

- [ ] **Step 1: Capture current standalone services**

Run:

```sh
docker compose --profile dev --profile dev-https --profile dev-proxy config --services
```

Expected before the change: output contains `worker-dev`, `worker-dev-https`,
and `worker-dev-proxy`.

- [ ] **Step 2: Remove all standalone worker service blocks**

Delete the complete `worker-dev`, `worker-dev-https`, and `worker-dev-proxy`
blocks from `compose.yaml`, including disabled healthchecks, duplicate
environment, volumes, networks, and `depends_on` entries.

Keep `x-app.command: ["serve"]`; each app service now starts both HTTP and
River. No worker-specific healthcheck is needed because the combined process
uses the app liveness endpoint.

- [ ] **Step 3: Correct the TLS certificate comment**

In `mise.toml`, change the SMTP certificate comment from app/worker callers to
app callers. Do not change generated hostnames: Mailpit service names remain
the same.

- [ ] **Step 4: Render all Compose profiles**

Run:

```sh
docker compose --profile dev --profile dev-https --profile dev-proxy config --services
```

Expected: output contains `app-dev`, `app-dev-https`, and `app-dev-proxy`, and
contains no service whose name starts with `worker-`.

Run:

```sh
docker compose --profile dev --profile dev-https --profile dev-proxy config >/dev/null
```

Expected: exit 0.

- [ ] **Step 5: Review Task 2 diff**

Run: `git diff --check -- compose.yaml mise.toml`

Expected: no output.

---

### Task 3: Update Operator and Architecture Documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/decisions/0003-server-owns-routing-with-injected-readiness.md`
- Create: `docs/decisions/0006-run-worker-with-http-server.md`

**Interfaces:**
- Consumes: Task 1 CLI behavior and Task 2 deployment topology.
- Produces: current operator instructions and an ADR for the combined lifecycle.

- [ ] **Step 1: Update README commands and topology**

Make these concrete changes:

- Quick Start says Compose starts PostgreSQL, Mailpit, pgAdmin, and the combined
  app service, not separate app and worker services.
- Direct-run example contains only `mise run app -- serve` and says it starts
  the HTTP API and background worker.
- Commands table describes `mise run app` as running `serve`, `healthcheck`, or
  `version`.
- `RIVER_MAX_WORKERS` notes that concurrency is per API replica.

- [ ] **Step 2: Record partial supersession in ADR-0003**

Add immediately below its status:

```markdown
**Partially superseded by:** ADR-0006 for the separate `serve` and `worker`
entrypoints. The routing and readiness decisions remain current.
```

Do not rewrite the historical context or original decision.

- [ ] **Step 3: Add ADR-0006**

Create `docs/decisions/0006-run-worker-with-http-server.md` with:

```markdown
# ADR-0006: Run the River worker with the HTTP server

## Status
Accepted

## Date
2026-08-15

## Context
The API and River worker used separate CLI subcommands and Compose services,
although both required the same database, migrations, store, mailer, and River
client. This duplicated configuration and startup paths. It also allowed the
API to accept jobs while its paired worker process was absent.

## Decision
The `serve` command starts River before starting HTTP. River owns its background
goroutines. If River cannot start, the process exits before accepting traffic.
On shutdown, HTTP stops first, River receives up to 10 seconds for graceful
completion, and the shared database pool closes last.

The standalone `worker` command and worker-only Compose services are removed.
Every API replica runs one River client, making `RIVER_MAX_WORKERS` a per-replica
concurrency limit.

## Alternatives Considered

### Keep separate API and worker processes
This preserves independent scaling but retains duplicate deployment and
configuration paths, contrary to the simplification goal.

### Wrap both components in a generic supervisor
River already manages its worker goroutines. A supervisor would add lifecycle
abstractions without another long-lived component that needs them.

## Consequences
- One command and one service start the complete application.
- Worker startup failure prevents a partially functional API process.
- Graceful shutdown preserves in-progress River jobs within a bounded window.
- API and worker capacity scale together; total worker concurrency is the
  per-replica setting multiplied by API replica count.
- Independently scaling HTTP and workers would require a future explicit
  architecture change.
```

- [ ] **Step 4: Scan active surfaces for stale standalone-worker instructions**

Run:

```sh
rg -n 'simplebank worker|mise run app -- worker|worker-dev|app \+ worker|serve.*worker.*subcommand' README.md compose.yaml cmd/app docs/decisions mise.toml
```

Expected: no stale operational instruction. Historical wording in ADR-0003 is
allowed because the supersession note preserves its context.

- [ ] **Step 5: Review Task 3 diff**

Run:

```sh
git diff --check -- README.md docs/decisions/0003-server-owns-routing-with-injected-readiness.md docs/decisions/0006-run-worker-with-http-server.md
```

Expected: no output.

---

### Task 4: Run Full Quality Gates

**Files:**
- Verify all files changed by Tasks 1-3.

**Interfaces:**
- Consumes: complete combined lifecycle, deployment, and documentation changes.
- Produces: executable evidence that the change is ready for review.

- [ ] **Step 1: Run all Go tests with the race detector**

Run: `go test -race ./...`

Expected: PASS.

- [ ] **Step 2: Run lint**

Run: `mise run golangci-lint`

Expected: PASS.

- [ ] **Step 3: Validate Compose rendering again**

Run:

```sh
docker compose --profile dev --profile dev-https --profile dev-proxy config >/dev/null
```

Expected: exit 0.

- [ ] **Step 4: Check final diff integrity**

Run: `git diff --check`

Expected: no output.

- [ ] **Step 5: Review acceptance criteria**

Confirm every criterion in
`docs/superpowers/specs/2026-08-15-combined-api-worker-lifecycle-design.md`
has matching code, tests, documentation, and validation evidence. Do not expand
scope to unrelated findings.
