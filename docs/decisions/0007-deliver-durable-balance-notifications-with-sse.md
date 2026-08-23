# ADR-0007: Deliver durable balance notifications with SSE

## Status
Accepted

## Date
2026-08-23

## Context
Account owners need timely balance-change notifications after transfers. The
delivery path must survive reconnects, offline clients, process restarts, and
multiple API replicas without making an ephemeral transport the source of
truth.

## Decision
`TransferTx` creates durable notification rows in the same transaction as the
balance changes. PostgreSQL `NOTIFY` is commit-gated acceleration for live
delivery, never the source of truth.

Every replica opens a dedicated PostgreSQL `LISTEN` connection and publishes
received notification IDs through a local, owner-scoped hub. This allows
cross-replica delivery without sticky sessions. Bounded per-subscriber queues
prevent slow subscribers from accumulating unbounded events.

Authenticated fetch-based SSE carries notification IDs only. On initial load
and reconnect, clients reconcile through authoritative REST notification and
account data rather than treating the stream as history.

The notification listener starts after migrations and before River and HTTP.
Shutdown stops HTTP, then River, then the listener, and finally the shared
database pool. Listener degradation does not fail readiness: readiness remains
a pool ping because durable reconciliation preserves correctness while live
delivery recovers.

## Alternatives Considered

### Sub-second polling
Rejected because continuous polling adds load even when no balances change and
provides weaker delivery timing.

### WebSockets
Rejected because notifications need only server-to-client invalidation;
bidirectional protocol and connection-management complexity is unnecessary.

### In-memory-only events
Rejected because they lose changes while clients or replicas are offline and
cannot deliver events across replicas.

## Consequences
- Notification history remains recoverable independently of `NOTIFY` and SSE.
- Each replica consumes one dedicated database connection for `LISTEN`.
- Live invalidations can be dropped under bounded backpressure without losing
  correctness because clients reconcile from durable REST state.
- Startup and shutdown explicitly preserve listener, River, HTTP, and pool
  dependencies without a generic service supervisor.
