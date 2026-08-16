# ADR-0005: Make transfers idempotent and enforce limits at safe boundaries

## Status
Accepted

## Date
2026-07-19

## Context
A transfer moves money between two accounts. Three failure modes make the naive
"validate then move" path unsafe:

- **Retries double-spend.** A client that resends a transfer after a lost
  response — a dropped connection, a impatient user clicking twice, a proxy
  retry — has no way to tell "the first one never landed" from "the first one
  landed but the reply was lost". Without a dedup key, the second request debits
  the account again.
- **Checks race the move (TOCTOU).** Validating currency, ownership, and balance
  against un-locked rows leaves a window where another transaction changes those
  rows before the money moves. The values the API checked are not the values the
  move runs against.
- **Limits need a consistent view.** A per-transfer cap can be checked from the
  request alone, but a rolling daily cap depends on the account's recent history,
  which must be read against the same locked rows the move updates or two
  concurrent transfers can each pass the cap and jointly exceed it.

Limits also are not a single number. USD cents and VND whole dong are not
comparable, so one global ceiling is meaningless across currencies.

## Decision
Every transfer carries a **client-generated idempotency key** (a required UUID
in the request body), scoped to the **source account** via the composite unique
constraint `(from_account_id, idempotency_key)`. Checks that depend on database
state run **inside the money-moving transaction** against locked rows; the
request-only per-transfer cap runs at the API edge before the transaction.

- **Idempotency.** The key is stored on the transfer row under the composite
  unique constraint `(from_account_id, idempotency_key)`. `TransferTx` takes a
  fast path: if a transfer already exists for the same source account and key,
  it replays the original result without touching balances. A concurrent request
  that wins the `CreateTransfer` race makes the loser hit the unique-constraint
  violation, which is resolved by replaying the winner. If a caller reuses the
  same source-scoped key with different immutable parameters (destination,
  amount, or asserted currency), the replay path returns `409 Conflict` instead
  of silently returning the wrong transfer. The key is the client's to own —
  the SPA holds one key stable across retries and rotates it only after a
  confirmed receipt (see the frontend transfer flow).
- **In-transaction re-validation.** Both accounts are locked with
  `SELECT … FOR UPDATE` in a deterministic order (smaller UUID first) to avoid
  deadlocks between opposing transfers. Currency is re-checked against the locked
  rows, closing the TOCTOU gap left by the API-layer pre-check.
- **Per-currency limits.** `TRANSFER_LIMITS` is a JSON object keyed by currency
  code, each with a `max_per_transfer` and an optional `daily` ceiling in that
  currency's own minor units. A zero or absent field disables that limit, so
  limits are opt-in per currency. The per-transfer cap is checked at the API
  edge (cheap, no DB); the rolling 24h daily cap is summed from the account's
  outgoing transfers inside the transaction, against the locked rows.
- **Balance guard in the UPDATE.** The debit runs as a guarded
  `UPDATE … SET balance = balance + $amount WHERE … AND balance + $amount >= 0
  RETURNING …`. A failed guard returns no row (insufficient balance); a bigint
  overflow returns a numeric-out-of-range error. The check and the write are one
  statement, so there is no read-then-write race.
- **Published policy endpoint.** `GET /api/v1/transfer-limits` exposes the
  per-currency ceilings (public — the numbers are policy, not secrets) so the
  SPA validates against the same limits the API enforces instead of hard-coding
  its own copy.

## Alternatives Considered

### Server-generated idempotency key, or no key at all
- Pros: nothing for the client to manage.
- Cons: a server-minted key changes on every request, so a retry after a lost
  response looks like a brand-new transfer and double-debits. Only a key the
  client holds stable across its own retries can collapse them.
- Rejected: it fails the exact case idempotency exists to handle.

### One global transfer limit instead of per-currency
- Pros: a single number to configure.
- Cons: a limit expressed in minor units cannot span currencies — `100000` is
  $1,000 but only ₫100,000. A shared cap is either absurdly high for one
  currency or absurdly low for another.
- Rejected: limits are only meaningful per currency.

### Enforce all limits and checks at the API layer only
- Pros: keeps `TransferTx` simple; no locking for the daily sum.
- Cons: currency and the daily total are read against un-locked rows, so a
  concurrent change or a second in-flight transfer can slip past. The daily cap
  in particular is a read-modify-write that two requests can both pass.
- Rejected: the daily cap and currency check must see the same locked rows the
  move updates.

### Application-level balance check (`SELECT` balance, then `UPDATE`)
- Pros: reads naturally in Go; easy to return a custom error before writing.
- Cons: the balance can change between the `SELECT` and the `UPDATE` — a classic
  lost-update race — letting an account overdraw under concurrency.
- Rejected: a single guarded `UPDATE` makes the check and the write atomic.

## Consequences
- A transfer is safe to retry: the same source-scoped key always yields the
  same result, and balances move at most once per `(from_account_id,
  idempotency_key)` pair.
- Concurrent transfers between the same two accounts cannot deadlock (fixed lock
  order) and cannot overdraw or jointly bust a daily cap (locked-row checks).
- Limits are configuration, not code: operators tune ceilings per currency via
  `TRANSFER_LIMITS` with no rebuild, and the SPA reads the live policy from
  `/transfer-limits`. A currency with no entry is unlimited by design.
- The transfer request contract gains a required `idempotency_key`; clients that
  omit it are rejected at validation. This is a deliberate, breaking change to
  the endpoint in exchange for retry safety.
- The daily-cap sum runs inside the hot transaction. It is a single indexed
  aggregate over the trailing window and only when a `daily` limit is set for the
  currency, so disabled-by-default keeps the common path free of it.
