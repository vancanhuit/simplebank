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
