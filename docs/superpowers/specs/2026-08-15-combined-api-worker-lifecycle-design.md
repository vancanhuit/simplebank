# Combined API and Worker Lifecycle Design

**Date:** 2026-08-15
**Status:** Approved

## Objective

Run River background jobs in the same process as the HTTP API. Remove the
standalone `worker` CLI subcommand and its duplicate Compose services while
preserving deterministic startup, graceful shutdown, and startup migrations.

## Current State

The binary exposes separate `serve` and `worker` subcommands. Both build the
same dependency graph and run the same migrations, but Compose starts separate
containers for them in each development profile. The API process constructs a
River client for enqueueing jobs without starting that client; the worker
process starts the client and does not expose HTTP health endpoints.

## Decision

The `serve` command owns one process lifecycle containing both components:

1. Build the shared dependency graph and run domain and River migrations.
2. Construct the API server and register the embedded SPA.
3. Start the River client.
4. Start the HTTP listener.
5. On shutdown or HTTP failure, stop River gracefully before closing the
   database pool.

River's `Client.Start` starts its job-fetching and execution loops in background
goroutines and returns after startup. The application therefore does not add a
wrapper goroutine or a generic supervisor. Starting River synchronously before
the HTTP listener guarantees that a worker startup failure prevents the API
from accepting traffic.

## Lifecycle and Error Handling

River starts with a background context. Canceling the context passed to
`Client.Start` performs a hard stop, while this application requires the
existing graceful behavior. The process signal context controls the blocking
HTTP server. After the HTTP server returns, the application calls
`Client.Stop` with a 10-second timeout, then closes the shared database pool.

The command returns:

- the River startup error when the worker cannot start; HTTP does not listen;
- the HTTP server error after attempting graceful worker shutdown;
- the River shutdown error when shutdown exceeds its deadline or otherwise
  fails; or
- both errors when HTTP serving and worker shutdown fail.

A small lifecycle helper may define the consumer-side `Start` and `Stop`
contract so ordering and failure behavior can be tested without PostgreSQL.
It must not introduce a general-purpose process framework.

## CLI and Deployment

The CLI retains `serve`, `healthcheck`, and `version`. The `worker` command and
its action are removed.

Compose removes `worker-dev`, `worker-dev-https`, and `worker-dev-proxy`.
Each corresponding app service runs both HTTP and River workloads. This also
removes duplicated dependency configuration and the startup ordering that was
needed between app and worker containers.

Every API replica runs one River client. River coordinates jobs through
PostgreSQL, so multiple replicas can process the queue safely. Consequently,
`RIVER_MAX_WORKERS` is a per-replica limit and total potential concurrency is
the configured value multiplied by the number of API replicas.

## Documentation

Update the README quick start, direct-run example, command description, and
configuration note to describe the combined process. Add an ADR for the new
lifecycle and mark the shared-entrypoint portion of ADR-0003 as superseded.
Historical implementation plans and design documents remain unchanged.

## Testing and Verification

Focused tests cover these invariants:

- the CLI no longer exposes `worker`;
- a River startup failure prevents HTTP startup;
- successful River startup is paired with graceful stop after HTTP returns;
- HTTP and River shutdown errors are preserved.

Verification runs the focused command-package tests with the race detector,
the full Go test suite, lint, and `docker compose config` for all profiles.

## Alternatives Rejected

### Run a worker lifecycle function in `errgroup`

River already owns its background goroutines. A wrapper goroutine would mostly
wait for cancellation and could let HTTP bind before River reports a startup
failure. The extra scheduling and cancellation paths do not improve behavior.

### Introduce a generic service supervisor

A supervisor could coordinate future long-lived components, but the current
application has only HTTP and River. A generic abstraction would add more API
surface and tests than this lifecycle requires.

### Keep the standalone worker command as an option

Supporting both combined and standalone modes would preserve duplicate
deployment paths and configuration. It conflicts with the goal of simplifying
the architecture and makes capacity semantics less obvious.

## Acceptance Criteria

- `simplebank serve` starts River before accepting HTTP traffic.
- Worker startup failure makes `serve` exit non-zero.
- SIGINT and SIGTERM stop HTTP and then give River up to 10 seconds to finish
  in-progress jobs.
- `simplebank worker` is no longer a valid command.
- Development Compose profiles contain no standalone worker service.
- README and ADRs describe the combined process and per-replica concurrency.
- Focused tests, full Go tests, lint, and Compose configuration validation pass.
