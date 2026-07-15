# ADR-0003: Server owns HTTP routing with an injected readiness probe

## Status
Accepted

## Date
2026-07-15

## Context
`api.Server` builds and holds the echo router and registers all application
routes. Kubernetes-style health endpoints are split by nature:

- **liveness** (`/livez`) reports only that the process is up.
- **readiness** (`/readyz`) reports whether the service can serve traffic, which
  for this service means the database is reachable.

Previously `Server.Handler()` returned the concrete `*echo.Echo` *specifically
so* `cmd/app/main.go` could register `/readyz` on it after construction, using
the pgx pool for the ping. That leaked the framework type through the Server's
interface and split the health concern across two modules: `/livez` inside the
Server, `/readyz` outside it. It also duplicated dependency wiring across the
`serve` and `worker` entrypoints.

## Decision
1. `NewServer` accepts an injected readiness probe, `func(context.Context) error`.
   The Server registers and owns `/readyz` (wrapping the probe in a 2-second
   timeout) alongside `/livez`. A nil probe defaults to a no-op that always
   reports ready, which keeps tests simple.
2. `Server.Handler()` returns `http.Handler` instead of `*echo.Echo`, hiding the
   framework. `echo.StartConfig.Start` already accepts `http.Handler`, so main
   passes the handler directly.
3. Shared dependency assembly is extracted into `buildApp` in `cmd/app/main.go`,
   which opens the pool, runs migrations, and constructs the store, mailer, and
   river client in order (closing the pool on any error). Both `serve` and
   `worker` use it.

## Alternatives Considered

### Keep registering `/readyz` in main on a returned `*echo.Echo`
- Pros: no signature change.
- Cons: leaks echo through the Server interface; the readiness route and its
  timeout live outside the module that owns routing; blocks narrowing
  `Handler()`.
- Rejected: the health concern belongs with the routing it is part of.

### Move server startup (`echo.StartConfig.Start`) into the Server too
- Pros: the Server would own its full lifecycle.
- Cons: broader change; couples the Server to process-lifecycle concerns that
  main already coordinates with signal handling.
- Rejected for now: injecting the readiness probe is enough to fix the seam;
  lifecycle can move later if it earns its keep.

## Consequences
- All routing and health endpoints live in one module; callers learn less about
  the framework (they get an `http.Handler`).
- Readiness is unit-testable by supplying a fake probe — see
  `TestReadyzReady` / `TestReadyzUnavailable` — instead of needing a live pool.
- `buildApp` gives the two entrypoints one ordered assembly path; adding a new
  entrypoint reuses it instead of duplicating the wiring and its ordering
  invariants (migrate before serve).
