---
name: postgres-transaction-design
description: explain, design, review, or implement postgresql transactions and acid-safe data workflows in golang. use for pgx, sqlc, repository or store transaction boundaries, atomic updates, constraints, isolation levels, row locks, optimistic concurrency, deadlock prevention, serialization retries, ledger or transfer logic, transactional outbox design, and diagnosing race conditions or cross-system consistency problems.
---

# PostgreSQL Transaction Design

Use this skill to design correct transaction boundaries and concurrency-safe PostgreSQL workflows for Go services.

## Workflow

1. State the business invariant before choosing SQL or an isolation level.
2. Identify every read, write, constraint, lock, and external side effect involved.
3. Keep one transaction aligned with one local database business operation.
4. Prefer database constraints and atomic SQL over application-side read-modify-write logic.
5. Choose the weakest isolation and locking strategy that safely protects the invariant.
6. Define expected retryable failures and make the entire transaction callback safe to rerun.
7. Separate database atomicity from cross-system consistency.
8. Include concurrency tests or deterministic race scenarios when correctness depends on interleaving.

## ACID interpretation

- Atomicity: all database changes in the transaction commit together or none do.
- Consistency: constraints and transaction logic preserve declared invariants.
- Isolation: concurrent transactions cannot produce forbidden outcomes.
- Durability: a successful commit survives ordinary process and host failures, subject to PostgreSQL and storage configuration.

Do not describe consistency as automatic knowledge of all business rules. Require important invariants to be encoded through constraints, atomic statements, locks, or serializable transactions.

## SQL design priorities

Prefer, in order:

1. schema constraints such as primary keys, foreign keys, unique constraints, `NOT NULL`, checks, and exclusion constraints
2. one atomic statement such as conditional `UPDATE ... RETURNING`
3. pessimistic row locking with deterministic lock order
4. optimistic concurrency using a version column
5. stronger isolation such as Repeatable Read or Serializable

Avoid a separate `SELECT`, application calculation, and unconditional `UPDATE` when one conditional update can enforce the invariant atomically.

Example shape:

```sql
UPDATE accounts
SET balance = balance - $1
WHERE id = $2
  AND balance >= $1
RETURNING balance;
```

## Go and pgx rules

- Accept and propagate `context.Context`.
- Begin the transaction, immediately defer rollback, perform work, then commit.
- Ignore the expected rollback-after-commit error.
- Use `sqlc` queries rebound with `Queries.WithTx(tx)`.
- Keep `pgx.Tx` out of HTTP handlers and domain entities.
- Keep transactions short and free from slow remote calls.
- Translate PostgreSQL errors into stable application errors.
- Retry the entire transaction for SQLSTATE `40001` or `40P01`, not only the failed statement.
- Ensure retry callbacks do not send email, publish messages, charge providers, or mutate external systems directly.

## Isolation guidance

- Read Committed: use for most workloads with atomic SQL, constraints, or explicit row locks.
- Repeatable Read: use when a stable snapshot is required; handle serialization-style conflicts.
- Serializable: use when cross-row predicates or write skew cannot be safely protected otherwise; expect and retry `40001`.

PostgreSQL does not permit dirty reads. Treat requested Read Uncommitted behavior as Read Committed.

## Locking guidance

Use `SELECT ... FOR UPDATE` when a later decision depends on the row value just read.

Acquire multiple locks in a stable order. For transfers, order account IDs consistently before locking to reduce deadlocks.

Use optimistic concurrency when conflicts are uncommon and a retry or conflict response is acceptable:

```sql
UPDATE resources
SET value = $1,
    version = version + 1
WHERE id = $2
  AND version = $3;
```

Treat zero affected rows as a concurrent modification.

## Cross-system boundary

A PostgreSQL transaction cannot atomically include:

- Valkey or Redis changes
- Kafka or queue publication
- HTTP calls
- S3 writes
- another independent database

Use a transactional outbox, idempotency, sagas, compensation, and reconciliation for multi-system workflows.

## Output structure

For a design or review request, produce:

1. invariant and failure scenario
2. unsafe interleaving
3. transaction boundary
4. constraints and SQL strategy
5. isolation or locking choice
6. pgx/sqlc implementation
7. retry and error mapping
8. concurrency test plan
9. cross-system consistency treatment

Read [references/transaction-checklist.md](references/transaction-checklist.md) for reviews, transfer or ledger examples, and concurrency troubleshooting.
