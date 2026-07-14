# Transaction Review Checklist

## Invariant

- What must always be true before and after commit?
- Can the invariant be encoded as a schema constraint?
- Does it span one row, several rows, or a predicate over a set of rows?

## Boundary

- Does the transaction represent one business operation?
- Are any HTTP, broker, object-storage, or cache calls inside the transaction?
- Could a transaction remain open during user interaction or long computation?

## Concurrency

- Can two requests read the same old state?
- Is there a lost-update risk?
- Can write skew violate a cross-row invariant?
- Are rows locked in a deterministic order?
- Is optimistic version checking sufficient?
- Is Serializable required?

## Failure handling

- What happens after a statement error?
- Is rollback guaranteed?
- Can commit itself fail?
- Are `40001` and `40P01` retried with a bounded policy?
- Is the entire callback safe to rerun?

## SQL and constraints

- Can a conditional update replace read-modify-write?
- Are `NOT NULL`, unique, foreign key, check, or exclusion constraints missing?
- Is the application distinguishing no row, conflict, and insufficient state correctly?

## Testing scenarios

- concurrent debit against the same account
- transfers locking the same accounts in opposite directions
- duplicate idempotency keys
- serialization failure followed by retry
- rollback after an intermediate statement failure
- commit failure or canceled context
- duplicate outbox delivery

## Transfer design example

A safe local transfer commonly includes:

1. validate positive amount and distinct accounts
2. lock both account rows in deterministic order, or use atomic debit plus protected credit
3. debit with an invariant that prevents negative balance
4. credit the destination
5. insert an immutable transfer or ledger record
6. optionally insert an outbox event in the same transaction
7. commit
8. publish externally only after commit through the outbox worker
