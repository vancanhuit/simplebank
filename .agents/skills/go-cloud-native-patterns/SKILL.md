---
name: go-cloud-native-patterns
description: design, review, explain, or implement cloud-native patterns in golang services. use for go http or grpc services that need resilience, scalability, observability, graceful lifecycle management, dependency boundaries, retries, timeouts, circuit breakers, bulkheads, rate limiting, backpressure, health checks, idempotency, outbox workflows, sagas, or deployment guidance for kubernetes and container platforms.
---

# Go Cloud-Native Patterns

Use this skill to turn cloud-native requirements into concrete Go designs, code, tests, and operational guidance.

## Workflow

1. Identify the service boundary, critical dependencies, stateful resources, and failure modes.
2. Separate baseline requirements from patterns that should only be added when a concrete risk justifies them.
3. Prefer standard-library primitives and small interfaces before introducing frameworks or middleware packages.
4. Show where each pattern belongs in the request path, background-worker path, or deployment topology.
5. Include code that propagates `context.Context`, bounds waiting, classifies errors, and supports tests.
6. Explain failure behavior, retry safety, observability signals, and operational tradeoffs.
7. End with a recommended adoption order rather than suggesting every pattern at once.

## Baseline design rules

Always consider these first:

- externalized, validated configuration
- stateless service instances
- constructor-based dependency injection
- context propagation through every blocking operation
- layered timeouts with inner deadlines shorter than outer deadlines
- graceful shutdown for HTTP servers and workers
- separate liveness, readiness, and startup probes
- structured logs to stdout
- RED metrics for request paths and useful business metrics
- stable error categories translated at transport boundaries
- short, explicit dependency interfaces

## Resilience rules

- Retry only transient failures.
- Bound retries by attempts, elapsed time, and parent context.
- Add jitter to exponential backoff.
- Retry an operation only when it is idempotent or protected by an idempotency key.
- Use circuit breakers around remote dependencies, not ordinary local calls.
- Use bulkheads to isolate slow dependencies or low-priority workloads.
- Use bounded queues and concurrency limits to create backpressure.
- Shed load deliberately with `429` or `503` before memory, goroutines, or connection pools are exhausted.

## Data and messaging rules

- Treat caches as disposable and keep a separate source of truth.
- Use a transactional outbox for database changes plus event publication.
- Make consumers idempotent because outbox delivery may repeat.
- Use sagas only when a workflow spans independent consistency boundaries.
- Use reconciliation for eventual repair of cross-system drift.
- Do not claim that one database transaction covers Valkey, Kafka, S3, HTTP calls, or another database.

## Go implementation guidance

Prefer these primitives:

- `signal.NotifyContext` for process cancellation
- `http.Server.Shutdown` for graceful HTTP shutdown
- `context.WithTimeout` for dependency deadlines
- channels or semaphores for bounded concurrency
- constructor functions for dependency injection
- `errors.Is` and `errors.As` for classification
- `log/slog` for structured logging
- OpenTelemetry for traces when distributed call chains justify it

When showing code:

- keep handlers thin
- place business decisions in application services
- hide framework-specific types at transport boundaries
- hide persistence-specific errors behind repository or store boundaries
- make callbacks used in retries free from non-idempotent external side effects
- show cleanup and cancellation paths

## Health probe semantics

- Liveness answers whether the process is alive. Do not normally depend on PostgreSQL or remote services.
- Readiness answers whether the replica can safely receive traffic. Check only critical dependencies and use short deadlines.
- Startup answers whether initialization has completed. Use it to avoid premature liveness failures during slow startup.

## Output structure

For explanation or review requests, produce:

1. system context and failure assumptions
2. recommended patterns grouped as baseline, conditional, and advanced
3. request or worker flow
4. minimal Go implementation examples
5. test strategy
6. operational signals and failure behavior
7. adoption order and anti-patterns

For implementation requests, provide compilable code or focused patches, including tests when practical.

## Reference

Read [references/pattern-catalog.md](references/pattern-catalog.md) when the user asks for a pattern comparison, architecture review, or a broad implementation roadmap.
