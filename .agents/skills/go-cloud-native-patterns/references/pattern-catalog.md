# Pattern Catalog

## Baseline

| Pattern | Solves | Typical Go mechanism | Common mistake |
|---|---|---|---|
| Externalized configuration | Environment-specific deployment | typed config loaded once | reading environment variables throughout the codebase |
| Stateless replicas | Horizontal scaling and replacement | external durable state | process-local sessions or durable local files |
| Context propagation | Cancellation and deadline propagation | `context.Context` | replacing request context with `context.Background()` |
| Timeouts | Unbounded dependency waits | `context.WithTimeout`, client timeouts | identical timeout at every layer |
| Graceful shutdown | Safe replacement and termination | `signal.NotifyContext`, `Server.Shutdown` | stopping HTTP but leaking workers |
| Health probes | Traffic and lifecycle control | `/livez`, `/readyz`, `/startupz` | making liveness depend on the database |
| Structured logging | Machine-readable operations | `log/slog` JSON handler | high-cardinality or secret fields |
| Metrics | Aggregate behavior | RED and business metrics | user IDs as labels |

## Conditional resilience

| Pattern | Use when | Avoid when |
|---|---|---|
| Retry | failures are transient and operations are safe to repeat | validation errors, permanent failures, non-idempotent side effects |
| Circuit breaker | a remote dependency repeatedly fails or stalls | local function calls or as a substitute for timeouts |
| Bulkhead | one dependency or workload can consume shared capacity | there is no meaningful isolation boundary |
| Rate limiting | protecting capacity or enforcing quotas | relying on per-replica limits for a global guarantee |
| Backpressure | producers can exceed processing capacity | using unbounded queues |
| Load shedding | preserving critical workloads during overload | treating every request as equal priority |
| Cache-aside | repeated reads are expensive and staleness is acceptable | correctness requires immediate consistency |

## Distributed workflow patterns

| Pattern | Use when | Key requirement |
|---|---|---|
| Transactional outbox | database update and event publication must not diverge | idempotent publisher and consumer |
| Saga | one business operation spans multiple services | explicit compensation and state transitions |
| Leader election | one worker should act across replicas | tolerate lock loss and duplicate execution |
| Reconciliation | eventual repair is acceptable | observable desired and actual state |
| Strangler | incrementally replacing a monolith | clear routing and ownership boundaries |

## Recommended adoption order

1. configuration, statelessness, context, timeouts
2. graceful shutdown and probes
3. structured logging, metrics, error taxonomy
4. bounded concurrency and request limits
5. idempotency and carefully scoped retries
6. circuit breakers, caching, and tracing when evidence justifies them
7. outbox, sagas, leader election, and service mesh only for genuine distributed-system needs
