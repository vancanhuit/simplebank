# Real-Time Balance Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist sender and recipient balance-change notifications and deliver authoritative account and notification updates to connected browsers in under one second.

**Architecture:** `TransferTx` creates two durable notification rows and queues PostgreSQL `NOTIFY` messages in the same transaction as the ledger and balance writes. Each application replica owns a PostgreSQL listener and a bounded owner-scoped hub; authenticated fetch-based SSE streams carry invalidation IDs, while the Svelte client always reconciles notifications and account balances from REST APIs.

**Tech Stack:** Go 1.27, PostgreSQL 18, pgx v5, sqlc 1.31.1, Echo v5, Svelte 5 runes, TypeScript 6, daisyUI 5, Vitest, Testing Library, Playwright.

**Spec:** `docs/superpowers/specs/2026-08-23-real-time-balance-notifications-design.md`

## Global Constraints

- Preserve source-first authorization, source-scoped idempotency, deterministic account-lock ordering, validation on locked rows, the rolling daily limit, guarded balance updates, and ledger consistency.
- Keep the existing `UNIQUE (owner, currency)` account invariant; do not add a same-owner transfer case.
- Money remains `int64` minor units and JSON-safe balances remain between `0` and `9007199254740991`.
- Notification rows and PostgreSQL notification messages must commit or roll back atomically with the transfer.
- A repeated or concurrent idempotent transfer creates no additional notification rows or live messages.
- Startup order is migrations -> notification listener -> River -> HTTP; shutdown order is HTTP -> River -> notification listener -> database pool.
- The SSE stream is an authenticated invalidation channel, not a source of account or notification truth.
- Every notification query and mutation is scoped by the authenticated username in SQL.
- Frontend responses and stream work started before logout must not repopulate the next session.
- Initial load and recovery reconciliation never produce historical toasts; only notifications first discovered from a live invalidation do.
- Do not add WebSockets, polling, browser push, email/SMS delivery, preferences, retention configuration, deletion, or archive behavior.
- Do not add a narrow handler repository interface; extend the existing wide `internal/db.Store` per ADR-0001.
- Never edit `internal/db/sqlc/` manually; run `mise run sqlc:generate` after changing migrations or queries.
- For every created or changed `.svelte` or `.svelte.ts` file, use the Svelte MCP documentation and run `svelte-autofixer` until it reports no errors or actionable suggestions.
- Preserve the existing daisyUI semantic colors, light/dark themes, 44px minimum targets, keyboard behavior, and responsive header.

## File Structure

### Database And Backend

- Create `internal/db/migrations/00006_balance_notifications.sql`: durable schema, constraints, and owner/history indexes.
- Create `internal/db/query/notifications.sql`: notification creation, publication, history, unread count, and owner-scoped read queries.
- Regenerate `internal/db/sqlc/models.go`, `internal/db/sqlc/notifications.sql.go`, and `internal/db/sqlc/querier.go`.
- Modify `internal/db/transfer_tx.go`: create and publish both notifications inside the existing money-moving transaction.
- Create `internal/db/notification_tx.go`: repeatable-read history snapshot and atomic read/count mutations.
- Modify `internal/db/store.go`: expose the handwritten notification operations through the existing wide store.
- Create `internal/db/notifications_query_test.go`: owner scope, stable cursor ordering, read state, and transactional publication integration tests.
- Modify `internal/db/transfer_tx_test.go` and `internal/db/transfer_safety_test.go`: transfer atomicity and idempotency coverage.
- Create `internal/notification/hub.go`: bounded owner-scoped in-process fan-out.
- Create `internal/notification/listener.go`: dedicated pgx `LISTEN` lifecycle and reconnect loop.
- Create `internal/notification/hub_test.go`, `internal/notification/listener_test.go`, and `internal/notification/listener_integration_test.go`.
- Create `internal/api/notification.go`: REST history/read handlers, cursor codec, and SSE handler.
- Create `internal/api/notification_test.go`: authorization, cursor, read, stream, keepalive, expiry, and cleanup tests.
- Modify `internal/api/server.go`, `internal/api/routes.go`, and `internal/api/user_test.go`: hub injection, stream timeout exception, routes, and fake store methods.
- Modify `cmd/app/main.go` and `cmd/app/main_test.go`: listener construction and ordered lifecycle.
- Create `docs/decisions/0007-deliver-durable-balance-notifications-with-sse.md` and modify `docs/decisions/README.md`: record the durable-row, PostgreSQL fan-out, SSE, and lifecycle decision.

### Frontend

- Modify `frontend/src/lib/api/types.ts`: notification API contracts.
- Modify `frontend/src/lib/api/client.ts` and its test: expose authenticated raw responses without duplicating refresh logic.
- Create `frontend/src/lib/api/sse.ts` and `sse.test.ts`: standards-compliant incremental SSE parser.
- Modify `frontend/src/lib/stores/accounts.svelte.ts` and its test: abortable refresh that preserves successful data.
- Create `frontend/src/lib/stores/notifications.svelte.ts` and its test: session lifecycle, reconciliation, reconnect, pagination, reads, toasts, and activity invalidation.
- Create `frontend/src/lib/components/NotificationItem.svelte`, `NotificationBell.svelte`, and `NotificationToasts.svelte`, with colocated component tests.
- Create `frontend/src/lib/pages/NotificationsPage.svelte` and `NotificationsPage.test.ts`.
- Modify `frontend/src/App.svelte`, `App.test.ts`, `AppHeader.svelte`, `AppHeader.test.ts`, `AccountHistoryPage.svelte`, and `AccountHistoryPage.test.ts`.
- Create `frontend/e2e/support/mock-api.ts` and `frontend/e2e/notifications.spec.ts`; modify `frontend/e2e/accessibility.spec.ts` to use shared mocks and include the bell.

---

### Task 1: Durable Notification Schema And Generated Queries

**Files:**
- Create: `internal/db/migrations/00006_balance_notifications.sql`
- Create: `internal/db/query/notifications.sql`
- Create: `internal/db/notifications_query_test.go`
- Regenerate: `internal/db/sqlc/models.go`
- Regenerate: `internal/db/sqlc/notifications.sql.go`
- Regenerate: `internal/db/sqlc/querier.go`

**Interfaces:**
- Produces generated `sqlcdb.Notification` and `CreateNotification`, `PublishNotification`, `ListNotifications`, `CountUnreadNotifications`, `MarkNotificationRead`, and `MarkAllNotificationsRead` methods.
- `CreateNotification` consumes only `transfer_id`, `account_id`, and `direction`; SQL derives owner, amount, currency, and post-transfer balance from committed transaction rows.

- [ ] **Step 1: Write the failing migration and query integration test**

Create an integration test that uses raw SQL first, so it fails before the migration exists:

```go
//go:build integration

package store

import (
    "testing"
    "uuid"
)

func TestNotificationsSchemaEnforcesTransferAccountUniqueness(t *testing.T) {
    sender := createTestUser(t)
    recipient := createTestUser(t)
    from := createTestAccount(t, sender.Username)
    to := createTestAccount(t, recipient.Username)
    transfer, err := testStore.CreateTransfer(t.Context(), sqlcdb.CreateTransferParams{
        FromAccountID: from.ID, ToAccountID: to.ID, Amount: 25, IdempotencyKey: uuid.New(),
    })
    if err != nil {
        t.Fatal(err)
    }

    const insert = `INSERT INTO notifications
        (owner, account_id, transfer_id, direction, amount, currency, balance)
        VALUES ($1, $2, $3, 'sent', 25, 'USD', 975)`
    if _, err := testPool.Exec(t.Context(), insert, sender.Username, from.ID, transfer.ID); err != nil {
        t.Fatal(err)
    }
    if _, err := testPool.Exec(t.Context(), insert, sender.Username, from.ID, transfer.ID); err == nil {
        t.Fatal("duplicate transfer/account notification succeeded")
    }
}
```

Import `sqlcdb` in the test. Keep test data isolated with the existing random user/account helpers.

- [ ] **Step 2: Run the test and confirm the missing relation failure**

Run:

```bash
mise run compose:test:up
go test -race -tags=integration ./internal/db -run '^TestNotificationsSchemaEnforcesTransferAccountUniqueness$'
mise run compose:test:down
```

Expected: FAIL because `notifications` does not exist. Always run the down command even if the test fails.

- [ ] **Step 3: Add migration `00006_balance_notifications.sql`**

Use this schema and reversible down migration:

```sql
-- +goose Up
CREATE TABLE notifications (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    owner       text NOT NULL REFERENCES users(username),
    account_id  uuid NOT NULL REFERENCES accounts(id),
    transfer_id uuid NOT NULL REFERENCES transfers(id),
    direction   text NOT NULL CHECK (direction IN ('sent', 'received')),
    amount      bigint NOT NULL CHECK (amount > 0 AND amount <= 9007199254740991),
    currency    text NOT NULL,
    balance     bigint NOT NULL CHECK (balance BETWEEN 0 AND 9007199254740991),
    read_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (transfer_id, account_id)
);

CREATE INDEX idx_notifications_owner_history
ON notifications (owner, created_at DESC, id DESC);

CREATE INDEX idx_notifications_owner_unread
ON notifications (owner)
WHERE read_at IS NULL;

-- +goose Down
DROP TABLE notifications;
```

- [ ] **Step 4: Add exact sqlc queries**

Create `internal/db/query/notifications.sql` with:

```sql
-- name: CreateNotification :one
INSERT INTO notifications (owner, account_id, transfer_id, direction, amount, currency, balance)
SELECT a.owner, a.id, t.id, sqlc.arg(direction), t.amount, a.currency, a.balance
FROM accounts AS a
JOIN transfers AS t ON t.id = sqlc.arg(transfer_id)
WHERE a.id = sqlc.arg(account_id)
  AND (
    (sqlc.arg(direction) = 'sent' AND t.from_account_id = a.id)
    OR (sqlc.arg(direction) = 'received' AND t.to_account_id = a.id)
  )
RETURNING *;

-- name: PublishNotification :exec
SELECT pg_notify(
  'balance_notifications',
  json_build_object(
    'id', sqlc.arg(notification_id)::uuid,
    'owner', sqlc.arg(owner)::text
  )::text
);

-- name: ListNotifications :many
SELECT *
FROM notifications
WHERE owner = sqlc.arg(owner)
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR (created_at, id) < (
      sqlc.arg(cursor_created_at)::timestamptz,
      sqlc.arg(cursor_id)::uuid
    )
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: CountUnreadNotifications :one
SELECT count(*)::bigint
FROM notifications
WHERE owner = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :one
UPDATE notifications
SET read_at = COALESCE(read_at, now())
WHERE id = sqlc.arg(id) AND owner = sqlc.arg(owner)
RETURNING *;

-- name: MarkAllNotificationsRead :execrows
UPDATE notifications
SET read_at = now()
WHERE owner = $1 AND read_at IS NULL;
```

- [ ] **Step 5: Generate code and inspect only generated effects**

Run:

```bash
mise run sqlc:generate
git diff -- internal/db/sqlc/models.go internal/db/sqlc/notifications.sql.go internal/db/sqlc/querier.go
```

Expected: a `Notification` model and all six query methods exist; no unrelated generated query disappears. Use the generated parameter field names in later tasks instead of guessing.

- [ ] **Step 6: Complete schema/query tests and make them pass**

Add tests that prove:

```go
func TestCreateNotificationDerivesTransferData(t *testing.T)
func TestListNotificationsOwnerScopeAndStableCursor(t *testing.T)
func TestNotificationReadQueriesAreOwnerScopedAndIdempotent(t *testing.T)
```

In the stable-cursor test, set multiple rows to one timestamp with raw SQL, order UUIDs descending, request two rows using `(created_at, id)`, and assert the second page neither duplicates nor skips IDs. In the read test, attempt `MarkNotificationRead` with a different owner and assert `errors.Is(ClassifyError(err), ErrRecordNotFound)`.

Run:

```bash
mise run compose:test:up
go test -race -tags=integration ./internal/db -run '^(TestNotificationsSchema|TestCreateNotification|TestListNotifications|TestNotificationReadQueries)'
mise run compose:test:down
```

Expected: PASS.

- [ ] **Step 7: Commit schema, queries, tests, and generated code**

```bash
git add internal/db/migrations/00006_balance_notifications.sql internal/db/query/notifications.sql internal/db/notifications_query_test.go internal/db/sqlc
git commit -m "feat(db): add durable balance notifications"
```

### Task 2: Atomic Transfer Notification Creation

**Files:**
- Modify: `internal/db/transfer_tx.go:83-125`
- Modify: `internal/db/transfer_tx_test.go`
- Modify: `internal/db/transfer_safety_test.go:17-154`

**Interfaces:**
- Consumes generated `CreateNotification` and `PublishNotification` from Task 1.
- Preserves `TransferTx(ctx context.Context, arg TransferTxParams) (TransferTxResult, error)` unchanged.
- Produces exactly two notification rows and two post-commit PostgreSQL messages for each newly committed transfer.

- [ ] **Step 1: Write failing transfer notification tests**

Add `TestTransferTxCreatesNotifications` and assert all durable fields:

```go
rows, err := testStore.ListNotifications(t.Context(), sqlcdb.ListNotificationsParams{
    Owner: sender.Username, HasCursor: false, PageLimit: 10,
})
if err != nil {
    t.Fatal(err)
}
if len(rows) != 1 {
    t.Fatalf("sender notifications = %d, want 1", len(rows))
}
got := rows[0]
if got.AccountID != from.ID || got.TransferID != result.Transfer.ID ||
    got.Direction != "sent" || got.Amount != amount || got.Currency != currency.USD ||
    got.Balance != result.FromAccount.Balance {
    t.Fatalf("unexpected sender notification: %+v", got)
}
```

Repeat for the recipient with direction `received` and `result.ToAccount.Balance`.

Extend `TestTransferTxIdempotent` and `TestTransferTxConcurrentSameKey` with:

```go
var notificationCount int
if err := testPool.QueryRow(t.Context(),
    `SELECT count(*) FROM notifications WHERE transfer_id = $1`, transferID,
).Scan(&notificationCount); err != nil {
    t.Fatal(err)
}
if notificationCount != 2 {
    t.Fatalf("notifications = %d, want 2", notificationCount)
}
```

Extend an existing failed-transfer test to assert zero notification rows for its account IDs.

- [ ] **Step 2: Run focused integration tests and verify failure**

```bash
mise run compose:test:up
go test -race -tags=integration ./internal/db -run '^(TestTransferTxCreatesNotifications|TestTransferTxIdempotent|TestTransferTxConcurrentSameKey|TestTransferTxInsufficientBalance)$'
mise run compose:test:down
```

Expected: new notification assertions FAIL with zero rows.

- [ ] **Step 3: Insert and publish both rows inside `TransferTx`**

After both guarded balance updates succeed and before returning from the existing `execTx` callback, add:

```go
fromNotification, err := q.CreateNotification(ctx, sqlcdb.CreateNotificationParams{
    TransferID: result.Transfer.ID,
    AccountID:  result.FromAccount.ID,
    Direction:  "sent",
})
if err != nil {
    return ClassifyError(err)
}
if err := q.PublishNotification(ctx, sqlcdb.PublishNotificationParams{
    NotificationID: fromNotification.ID,
    Owner:          fromNotification.Owner,
}); err != nil {
    return ClassifyError(err)
}

toNotification, err := q.CreateNotification(ctx, sqlcdb.CreateNotificationParams{
    TransferID: result.Transfer.ID,
    AccountID:  result.ToAccount.ID,
    Direction:  "received",
})
if err != nil {
    return ClassifyError(err)
}
if err := q.PublishNotification(ctx, sqlcdb.PublishNotificationParams{
    NotificationID: toNotification.ID,
    Owner:          toNotification.Owner,
}); err != nil {
    return ClassifyError(err)
}
```

Use the actual generated field names from Task 1. Do not add these calls to the idempotency fast path, `replayTransfer`, or unique-violation replay.

- [ ] **Step 4: Prove `NOTIFY` is commit-gated**

In `internal/db/notifications_query_test.go`, acquire a dedicated pgx connection, run `LISTEN balance_notifications`, and verify:

1. A manual transaction that calls `PublishNotification` and then rolls back yields no message within 200ms.
2. A successful `TransferTx` yields two messages.
3. Each JSON payload decodes into exactly `id` and `owner`, and both owners match the transfer accounts.

Use `context.WithTimeout` around `WaitForNotification`; treat `context.DeadlineExceeded` as the expected rollback result.

- [ ] **Step 5: Run all transfer/database notification tests**

```bash
mise run compose:test:up
go test -race -tags=integration ./internal/db -run '^(TestTransferTx|TestNotification)'
mise run compose:test:down
```

Expected: PASS, including rollback and idempotent replay assertions.

- [ ] **Step 6: Commit transfer atomicity**

```bash
git add internal/db/transfer_tx.go internal/db/transfer_tx_test.go internal/db/transfer_safety_test.go internal/db/notifications_query_test.go
git commit -m "feat(db): record transfer notifications atomically"
```

### Task 3: Owner-Scoped Notification Snapshot And Read Transactions

**Files:**
- Create: `internal/db/notification_tx.go`
- Modify: `internal/db/store.go:13-38`
- Modify: `internal/db/notifications_query_test.go`

**Interfaces:**
- Produces:

```go
type ListNotificationsPageParams struct {
    Owner           string
    HasCursor       bool
    CursorCreatedAt time.Time
    CursorID        uuid.UUID
    Limit           int32
}

type ListNotificationsPageResult struct {
    Notifications []sqlcdb.Notification
    UnreadCount   int64
    HasMore       bool
}

func (s *SQLStore) ListNotificationsPage(context.Context, ListNotificationsPageParams) (ListNotificationsPageResult, error)
func (s *SQLStore) MarkNotificationReadTx(context.Context, string, uuid.UUID) (int64, error)
func (s *SQLStore) MarkAllNotificationsReadTx(context.Context, string) (int64, error)
```

- Later API tasks consume these methods through `store.Store`.

- [ ] **Step 1: Write failing store transaction tests**

Add:

```go
func TestListNotificationsPageReturnsSnapshotAndHasMore(t *testing.T)
func TestMarkNotificationReadTxReturnsAuthoritativeCount(t *testing.T)
func TestMarkAllNotificationsReadTxReturnsAuthoritativeCount(t *testing.T)
```

For history, create three notifications, request `Limit: 2`, and assert two rows, `HasMore == true`, and the unread count includes all three. For individual read, seed two unread rows, mark one, assert count `1`, repeat and assert count stays `1`. Try the same ID with another owner and assert `ErrRecordNotFound`. For bulk read, seed another owner's unread row and assert only the requested owner's count reaches zero.

- [ ] **Step 2: Run focused tests and verify compile failure**

```bash
mise run compose:test:up
go test -race -tags=integration ./internal/db -run '^(TestListNotificationsPage|TestMarkNotification)'
mise run compose:test:down
```

Expected: FAIL because handwritten store types and methods do not exist.

- [ ] **Step 3: Add an options-aware transaction helper**

Refactor `internal/db/store.go` without changing existing callers:

```go
func (s *SQLStore) execTx(ctx context.Context, fn func(*sqlcdb.Queries) error) error {
    return s.execTxOptions(ctx, pgx.TxOptions{}, fn)
}

func (s *SQLStore) execTxOptions(
    ctx context.Context,
    opts pgx.TxOptions,
    fn func(*sqlcdb.Queries) error,
) error {
    return pgx.BeginTxFunc(ctx, s.connPool, opts, func(tx pgx.Tx) error {
        return fn(sqlcdb.New(tx))
    })
}
```

- [ ] **Step 4: Implement the three owner-scoped operations**

Use `pgx.RepeatableRead` for history and `Limit+1` for `HasMore`:

```go
func (s *SQLStore) ListNotificationsPage(ctx context.Context, arg ListNotificationsPageParams) (ListNotificationsPageResult, error) {
    var result ListNotificationsPageResult
    err := s.execTxOptions(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead}, func(q *sqlcdb.Queries) error {
        rows, err := q.ListNotifications(ctx, sqlcdb.ListNotificationsParams{
            Owner: arg.Owner, HasCursor: arg.HasCursor,
            CursorCreatedAt: arg.CursorCreatedAt, CursorID: arg.CursorID,
            PageLimit: arg.Limit + 1,
        })
        if err != nil {
            return ClassifyError(err)
        }
        result.UnreadCount, err = q.CountUnreadNotifications(ctx, arg.Owner)
        if err != nil {
            return ClassifyError(err)
        }
        if len(rows) > int(arg.Limit) {
            result.HasMore = true
            rows = rows[:arg.Limit]
        }
        result.Notifications = rows
        return nil
    })
    return result, err
}
```

Normalize limits to `1..100` at the API edge; this store receives a validated limit. Implement read transactions as owner-scoped update followed by `CountUnreadNotifications` in the same default transaction. `MarkNotificationReadTx` classifies `pgx.ErrNoRows` to `ErrRecordNotFound`; `MarkAllNotificationsReadTx` is idempotent and always returns the final count.

- [ ] **Step 5: Extend `Store` and pass integration tests**

Add the three methods to `Store`, run:

```bash
mise run compose:test:up
go test -race -tags=integration ./internal/db -run '^(TestListNotificationsPage|TestMarkNotification|TestMarkAllNotifications)'
mise run compose:test:down
```

Expected: PASS under `-race`.

- [ ] **Step 6: Commit the store operations**

```bash
git add internal/db/store.go internal/db/notification_tx.go internal/db/notifications_query_test.go
git commit -m "feat(db): add notification history and read operations"
```

### Task 4: Bounded Owner-Scoped Subscriber Hub

**Files:**
- Create: `internal/notification/hub.go`
- Create: `internal/notification/hub_test.go`

**Interfaces:**
- Produces:

```go
type Hub struct
func NewHub() *Hub
func (h *Hub) Subscribe(owner string) (<-chan uuid.UUID, func())
func (h *Hub) Publish(owner string, id uuid.UUID)
```

- The listener publishes owner/ID pairs; API streams subscribe by authenticated owner.

- [ ] **Step 1: Write failing hub tests**

Create tests:

```go
func TestHubPublishesOnlyToOwner(t *testing.T)
func TestHubSlowSubscriberDoesNotBlock(t *testing.T)
func TestHubUnsubscribeRemovesSubscriber(t *testing.T)
func TestHubConcurrentPublishAndUnsubscribe(t *testing.T)
```

Use this core assertion:

```go
alice, unsubscribeAlice := hub.Subscribe("alice")
t.Cleanup(unsubscribeAlice)
bob, unsubscribeBob := hub.Subscribe("bob")
t.Cleanup(unsubscribeBob)

id := uuid.New()
hub.Publish("alice", id)
if got := <-alice; got != id {
    t.Fatalf("alice received %s, want %s", got, id)
}
select {
case got := <-bob:
    t.Fatalf("bob received alice event %s", got)
default:
}
```

Fill one subscriber channel, publish one additional event from a goroutine, and assert it returns within 100ms. Run concurrent subscribe/unsubscribe/publish loops under `-race`.

- [ ] **Step 2: Run tests and verify missing package failure**

```bash
go test -race ./internal/notification -run '^TestHub'
```

Expected: FAIL because `Hub` does not exist.

- [ ] **Step 3: Implement minimal bounded fan-out**

Use a mutex and owner-indexed subscriber map:

```go
const subscriberBuffer = 16

type Hub struct {
    mu          sync.Mutex
    nextID      uint64
    subscribers map[string]map[uint64]chan uuid.UUID
}
```

`Subscribe` creates a buffered channel and an idempotent unsubscribe closure. Do not close subscriber channels: context cancellation controls consumers, and leaving channels open removes the send-on-closed race. `Publish` holds the mutex while attempting non-blocking sends:

```go
select {
case subscriber <- id:
default:
}
```

- [ ] **Step 4: Run hub tests repeatedly under the race detector**

```bash
go test -race -count=20 ./internal/notification -run '^TestHub'
```

Expected: PASS with no race reports or blocked test.

- [ ] **Step 5: Commit the hub**

```bash
git add internal/notification/hub.go internal/notification/hub_test.go
git commit -m "feat(notification): add bounded subscriber hub"
```

### Task 5: PostgreSQL LISTEN Lifecycle And Reconnect

**Files:**
- Create: `internal/notification/listener.go`
- Create: `internal/notification/listener_test.go`
- Create: `internal/notification/listener_integration_test.go`

**Interfaces:**
- Consumes `*pgx.ConnConfig` and `*Hub`.
- Produces lifecycle methods:

```go
func NewListener(config *pgx.ConnConfig, hub *Hub) *Listener
func (l *Listener) Start(context.Context) error
func (l *Listener) Stop(context.Context) error
```

- `Start` establishes the initial connection and `LISTEN` synchronously before returning, then starts the wait loop. Later failures reconnect with delays from 100ms up to 5s.

- [ ] **Step 1: Write deterministic listener unit tests**

Define an unexported test seam in the production design:

```go
type listenerConn interface {
    Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
    WaitForNotification(context.Context) (*pgconn.Notification, error)
    Close(context.Context) error
}

type connectFunc func(context.Context, *pgx.ConnConfig) (listenerConn, error)
```

Tests instantiate a listener with a fake `connect` function and assert:

```go
func TestListenerStartListensBeforeReturning(t *testing.T)
func TestListenerPublishesDecodedNotification(t *testing.T)
func TestListenerIgnoresMalformedPayload(t *testing.T)
func TestListenerReconnectsAfterWaitFailure(t *testing.T)
func TestListenerBackoffIsBounded(t *testing.T)
func TestListenerStopCancelsWait(t *testing.T)
```

Inject a sleep function in tests so reconnect bounds are asserted without real five-second waits.

- [ ] **Step 2: Run unit tests and verify failure**

```bash
go test -race ./internal/notification -run '^TestListener'
```

Expected: FAIL because `Listener` does not exist.

- [ ] **Step 3: Implement initial connection and reconnect loop**

Copy the config in `NewListener` with `config.Copy()`. `Start` must:

1. Reject a second start.
2. Create a child context and cancellation function.
3. Call `pgx.ConnectConfig` through the seam.
4. Execute `LISTEN balance_notifications`.
5. Return an error and close the connection if either initial step fails.
6. Spawn a goroutine only after the initial `LISTEN` succeeds.

The goroutine decodes only this payload:

```go
type payload struct {
    ID    uuid.UUID `json:"id"`
    Owner string    `json:"owner"`
}
```

Reject a nil UUID or empty owner. After a wait failure, close the connection, reconnect, execute `LISTEN`, and reset backoff after a successful listen. Backoff waits must select on the listener context. `Stop` cancels and waits for the goroutine or returns the supplied shutdown context error.

- [ ] **Step 4: Add the cross-replica integration test**

The `internal/notification` package does not share `internal/db`'s private test pool. In the integration test, read `DB_SOURCE` with the same default used by `internal/db/main_test.go`, parse it with `pgx.ParseConfig`, and open one publisher connection with `pgx.ConnectConfig`. Create two hubs and listeners from independent `config.Copy()` values. Start both, execute one `pg_notify` through the publisher, and assert subscribers on both hubs receive the same UUID within one second:

```go
_, err := publisher.Exec(t.Context(), `SELECT pg_notify(
    'balance_notifications',
    json_build_object('id', $1::uuid, 'owner', $2::text)::text
)`, id, "alice")
```

Close the publisher and stop both listeners in `t.Cleanup` with bounded contexts. This test needs PostgreSQL but no migrated tables because it exercises only `LISTEN`/`NOTIFY`.

- [ ] **Step 5: Run listener unit and integration tests**

```bash
go test -race ./internal/notification -run '^TestListener'
mise run compose:test:up
go test -race -tags=integration ./internal/notification -run '^TestListenerCrossReplicaDelivery$'
mise run compose:test:down
```

Expected: PASS.

- [ ] **Step 6: Commit the listener**

```bash
git add internal/notification/listener.go internal/notification/listener_test.go internal/notification/listener_integration_test.go
git commit -m "feat(notification): listen for committed balance changes"
```

### Task 6: Notification REST API And Opaque Cursor

**Files:**
- Create: `internal/api/notification.go`
- Create: `internal/api/notification_test.go`
- Modify: `internal/api/routes.go:38-45`
- Modify: `internal/api/user_test.go:29-127`

**Interfaces:**
- Consumes Task 3 store methods.
- Produces authenticated routes:

```text
GET /api/v1/notifications?size=20&cursor=<opaque>
PUT /api/v1/notifications/:id/read
PUT /api/v1/notifications/read-all
```

- Produces JSON contracts:

```go
type notificationResponse struct {
    ID         uuid.UUID  `json:"id"`
    AccountID  uuid.UUID  `json:"account_id"`
    TransferID uuid.UUID  `json:"transfer_id"`
    Direction  string     `json:"direction"`
    Amount     int64      `json:"amount"`
    Currency   string     `json:"currency"`
    Balance    int64      `json:"balance"`
    ReadAt     *time.Time `json:"read_at"`
    CreatedAt  time.Time  `json:"created_at"`
}

type listNotificationsResponse struct {
    Notifications []notificationResponse `json:"notifications"`
    UnreadCount   int64                  `json:"unread_count"`
    NextCursor    *string                `json:"next_cursor"`
}

type notificationReadResponse struct {
    UnreadCount int64 `json:"unread_count"`
}
```

- [ ] **Step 1: Extend the API fake store and write failing handler tests**

Add function fields and forwarding methods to `fakeStore` for all three handwritten store methods. Add tests:

```go
func TestNotificationEndpointsRequireAuthentication(t *testing.T)
func TestListNotificationsUsesAuthenticatedOwner(t *testing.T)
func TestListNotificationsRejectsInvalidCursor(t *testing.T)
func TestListNotificationsReturnsStableNextCursor(t *testing.T)
func TestMarkNotificationReadUsesAuthenticatedOwner(t *testing.T)
func TestMarkAllNotificationsReadUsesAuthenticatedOwner(t *testing.T)
```

Use `mustIssueTokenPair(t, "alice").access` and set `Authorization: Bearer <token>`. In fake functions, fail the test unless owner equals `alice`. Return `store.ErrRecordNotFound` for a foreign notification and assert HTTP 404 without ownership detail.

- [ ] **Step 2: Run focused API tests and verify missing route failure**

```bash
go test -race ./internal/api -run '^Test(Notification|ListNotifications|MarkNotification)'
```

Expected: FAIL with 404 or missing symbols.

- [ ] **Step 3: Implement strict cursor encode/decode**

Use unpadded base64url JSON:

```go
type notificationCursor struct {
    CreatedAt time.Time `json:"created_at"`
    ID        uuid.UUID `json:"id"`
}
```

Decode with `json.Decoder`, reject unknown fields, trailing JSON, zero time, and nil UUID. Return HTTP 400 `invalid notification cursor` for every malformed value. Encode the last returned row only when `HasMore` is true.

- [ ] **Step 4: Implement list and read handlers**

List rules:

- Default size `20`, clamp to `1..100` by rejecting non-numeric values and using `20` for values below 1, `100` above 100, matching existing list conventions.
- Get username only from `authPayload(c)`.
- Convert nullable generated `read_at` into `*time.Time` in the response DTO.
- Do not expose `owner`.

Read rules:

- Parse notification UUID; malformed ID returns 400.
- Call `MarkNotificationReadTx(ctx, payload.Username, id)`.
- `PUT /notifications/read-all` calls `MarkAllNotificationsReadTx`.
- Return the authoritative unread count from each successful mutation.

- [ ] **Step 5: Register authenticated REST routes and pass tests**

Add under the existing authenticated group:

```go
auth.GET("/notifications", s.listNotifications)
auth.PUT("/notifications/:id/read", s.markNotificationRead)
auth.PUT("/notifications/read-all", s.markAllNotificationsRead)
```

Run:

```bash
go test -race ./internal/api -run '^Test(NotificationEndpoints|ListNotifications|MarkNotification|MarkAllNotifications)'
```

Expected: PASS.

- [ ] **Step 6: Commit REST API behavior**

```bash
git add internal/api/notification.go internal/api/notification_test.go internal/api/routes.go internal/api/user_test.go
git commit -m "feat(api): expose notification history and read state"
```

### Task 7: Authenticated SSE Notification Stream

**Files:**
- Modify: `internal/api/notification.go`
- Modify: `internal/api/notification_test.go`
- Modify: `internal/api/server.go:27-74`
- Modify: `internal/api/routes.go`
- Modify: `internal/api/health_test.go`, `internal/api/user_test.go`, and `cmd/app/main.go` constructor call sites

**Interfaces:**
- Consumes `*notification.Hub` from Task 4.
- Changes `NewServer` to:

```go
func NewServer(
    cfg config.Config,
    st store.Store,
    maker token.Maker,
    riverClient *river.Client[pgx.Tx],
    notificationHub *notification.Hub,
    readiness func(context.Context) error,
) (*Server, error)
```

- Produces `GET /api/v1/notifications/stream` with `event: notification` and UUID-only `data` frames.

- [ ] **Step 1: Write failing stream tests with a real HTTP test server**

Add:

```go
func TestNotificationStreamRequiresAuthentication(t *testing.T)
func TestNotificationStreamFiltersAuthenticatedOwner(t *testing.T)
func TestNotificationStreamEmitsKeepalive(t *testing.T)
func TestNotificationStreamStopsAtTokenExpiry(t *testing.T)
func TestNotificationStreamUnsubscribesOnDisconnect(t *testing.T)
```

Use `httptest.NewServer(s.Handler())`, an `http.Client`, and a request context. Read the body through `bufio.Reader`. Publish a Bob ID and Alice ID through the injected hub; assert only Alice's frame appears:

```text
event: notification
data: 018f...

```

Set an unexported `notificationKeepalive` server field to 10ms in tests. Issue a short-lived access token directly through the maker for the expiry test.

- [ ] **Step 2: Run stream tests and verify failure**

```bash
go test -race ./internal/api -run '^TestNotificationStream'
```

Expected: FAIL because the route and hub field do not exist.

- [ ] **Step 3: Inject the hub and exempt only the stream from 30s context timeout**

Add to `Server`:

```go
notificationHub       *notification.Hub
notificationKeepalive time.Duration
```

Default a nil hub to `notification.NewHub()` for existing isolated tests and default keepalive to `15 * time.Second`. Replace the global timeout middleware with:

```go
e.Use(middleware.ContextTimeoutWithConfig(middleware.ContextTimeoutConfig{
    Timeout: 30 * time.Second,
    Skipper: func(c *echo.Context) bool {
        return c.Request().URL.Path == "/api/v1/notifications/stream"
    },
}))
```

Pass `nil` for the new hub argument from existing unit-test constructors until a test needs an injected hub.

- [ ] **Step 4: Implement the SSE handler**

Handler rules:

1. Get `payload.Username` and require non-nil `payload.ExpiresAt`.
2. Create `streamCtx` with deadline `payload.ExpiresAt.Time`.
3. Subscribe to that username and defer unsubscribe.
4. Set `Content-Type: text/event-stream`, `Cache-Control: no-store`, `Connection: keep-alive`, and `X-Accel-Buffering: no`.
5. Write `: connected\n\n`, flush, then select over stream context, owner event channel, and keepalive ticker.
6. Before each write, call `http.NewResponseController(c.Response()).SetWriteDeadline(time.Now().Add(30 * time.Second))`; ignore only `http.ErrNotSupported`.
7. Write events with `fmt.Fprintf(writer, "event: notification\ndata: %s\n\n", id)` and comments with `: keepalive\n\n`.
8. Flush every frame and return nil on cancellation or a write failure after the response is committed.

Register:

```go
auth.GET("/notifications/stream", s.streamNotifications)
```

- [ ] **Step 5: Run API tests including timeout middleware regression**

```bash
go test -race ./internal/api -run '^(TestNotificationStream|TestSecurityHeaders|TestReadyz)'
```

Expected: PASS. Existing CSP keeps `connect-src 'self'`; no CSP change is needed.

- [ ] **Step 6: Commit SSE delivery**

```bash
git add internal/api/notification.go internal/api/notification_test.go internal/api/server.go internal/api/routes.go internal/api/health_test.go internal/api/user_test.go cmd/app/main.go
git commit -m "feat(api): stream notification invalidations"
```

### Task 8: Composition Root And Ordered Service Lifecycle

**Files:**
- Modify: `cmd/app/main.go:121-218`
- Modify: `cmd/app/main_test.go:20-124`
- Create: `docs/decisions/0007-deliver-durable-balance-notifications-with-sse.md`
- Modify: `docs/decisions/README.md`

**Interfaces:**
- Consumes `notification.NewHub`, `notification.NewListener`, and API hub injection.
- Replaces `workerLifecycle` with a shared two-method lifecycle contract:

```go
type serviceLifecycle interface {
    Start(context.Context) error
    Stop(context.Context) error
}
```

- Changes `runServices(ctx, listener, worker, serve)` to enforce the approved ordering.

- [ ] **Step 1: Rewrite lifecycle tests to assert exact order and failures**

Use a fake lifecycle with `name`, shared `[]string`, and configurable errors. Add:

```go
func TestRunServicesListenerStartFailurePreventsWorkerAndHTTP(t *testing.T)
func TestRunServicesWorkerStartFailureStopsListener(t *testing.T)
func TestRunServicesOrdersStartupAndShutdown(t *testing.T)
func TestRunServicesPreservesServerWorkerAndListenerErrors(t *testing.T)
```

The successful order assertion is exact:

```go
want := []string{
    "listener:start",
    "worker:start",
    "http:serve",
    "worker:stop",
    "listener:stop",
}
```

Assert worker and listener receive separate non-cancelled shutdown contexts, each with approximately ten seconds remaining.

- [ ] **Step 2: Run lifecycle tests and verify signature failure**

```bash
go test -race ./cmd/app -run '^TestRunServices'
```

Expected: FAIL until `runServices` accepts and orders both services.

- [ ] **Step 3: Construct notification dependencies after migrations**

Extend `appDeps`:

```go
notificationHub      *notification.Hub
notificationListener *notification.Listener
```

In `buildApp`, after migrations and store creation:

```go
hub := notification.NewHub()
listener := notification.NewListener(pool.Config().ConnConfig, hub)
```

Pass the same hub to `api.NewServer`. Keep readiness as `app.pool.Ping`.

- [ ] **Step 4: Implement ordered lifecycle without a generic supervisor**

`runServices` must:

1. Start listener with `context.WithoutCancel(ctx)`; on failure return `starting notification listener: %w`.
2. Start River; if it fails, stop the started listener and join both errors.
3. Serve HTTP only after both starts succeed.
4. Stop River with its own ten-second context.
5. Stop listener afterward with a fresh ten-second context.
6. Join HTTP, River-stop, and listener-stop errors.

The existing `defer app.pool.Close()` remains outside and therefore runs last.

- [ ] **Step 5: Run lifecycle and full composition tests**

```bash
mise run frontend:build
go test -race ./cmd/app -run '^(TestRunServices|TestConfigureServerTimeouts|TestNewCommand)'
go test -race ./internal/api
```

Expected: PASS.

- [ ] **Step 6: Record ADR-0007 and update the decision index**

Create an accepted ADR dated 2026-08-23 with these exact decisions:

- Durable notification rows are created in `TransferTx`; PostgreSQL `NOTIFY` is commit-gated acceleration, never the source of truth.
- Every replica uses a dedicated `LISTEN` connection and bounded local owner-scoped hub, allowing cross-replica delivery without sticky sessions.
- Authenticated fetch-based SSE carries notification IDs only; reconnect reconciliation reads authoritative REST data.
- The listener starts before River/HTTP and stops after River but before the pool; listener degradation does not fail readiness because durable recovery preserves correctness.
- Alternatives rejected: sub-second polling due continuous load and weaker timing, WebSockets due unnecessary bidirectional protocol complexity, and in-memory-only events because they lose offline changes and cannot cross replicas.

Add ADR 0007 as `Accepted` to the table in `docs/decisions/README.md`.

- [ ] **Step 7: Commit lifecycle wiring and ADR**

```bash
git add cmd/app/main.go cmd/app/main_test.go docs/decisions/0007-deliver-durable-balance-notifications-with-sse.md docs/decisions/README.md
git commit -m "feat(app): run notification listener with services"
```

### Task 9: Frontend Raw Response And SSE Parser

**Files:**
- Modify: `frontend/src/lib/api/types.ts`
- Modify: `frontend/src/lib/api/client.ts`
- Modify: `frontend/src/lib/api/client.test.ts`
- Create: `frontend/src/lib/api/sse.ts`
- Create: `frontend/src/lib/api/sse.test.ts`

**Interfaces:**
- Produces:

```ts
export type NotificationDirection = "sent" | "received";

export interface Notification {
  id: string;
  account_id: string;
  transfer_id: string;
  direction: NotificationDirection;
  amount: number;
  currency: Currency;
  balance: number;
  read_at: string | null;
  created_at: string;
}

export interface NotificationPage {
  notifications: Notification[];
  unread_count: number;
  next_cursor: string | null;
}

export interface NotificationReadResponse {
  unread_count: number;
}

export interface ServerSentEvent {
  event: string;
  data: string;
  id: string;
}

export function requestResponse(path: string, options?: RequestOptions): Promise<Response>;
export function consumeEventStream(
  response: Response,
  onEvent: (event: ServerSentEvent) => void,
  signal?: AbortSignal,
): Promise<void>;
```

- [ ] **Step 1: Write failing raw-response and parser tests**

In `client.test.ts`, assert `requestResponse` attaches bearer auth, retries once after a 401 through existing refresh, returns the successful body unread, and throws `ApiError` on a final non-2xx response.

In `sse.test.ts`, build a `ReadableStream<Uint8Array>` and enqueue deliberately split chunks:

```ts
controller.enqueue(encoder.encode(": connected\r\nev"));
controller.enqueue(encoder.encode("ent: notification\r\ndata: first\r\ndata: second\r\n\r\n"));
```

Assert one event with `event === "notification"`, `data === "first\nsecond"`, comments ignored, CRLF accepted, EOF dispatch handled, and abort rejects with `AbortError` without invoking further callbacks.

- [ ] **Step 2: Run focused frontend tests and verify failure**

```bash
mise run frontend:test -- src/lib/api/client.test.ts src/lib/api/sse.test.ts
```

Expected: FAIL because the exports do not exist.

- [ ] **Step 3: Refactor the API client without duplicating refresh behavior**

Move the current generation capture, send, 401 refresh, and retry flow into `requestResponse`. For a non-2xx response, decode its error body once and throw `ApiError`. Change `request<T>` to:

```ts
export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  return decode<T>(await requestResponse(path, options));
}
```

Do not parse a successful body in `requestResponse`; the stream consumer needs `response.body` intact.

- [ ] **Step 4: Implement the incremental SSE parser**

Use `response.body?.getReader()` and one `TextDecoder`. Normalize CRLF only at line boundaries, preserve incomplete trailing chunks, join repeated `data:` fields with `\n`, ignore comment lines beginning `:`, and dispatch on a blank line. Default event type to `message`; do not parse notification UUIDs in this transport helper.

Always release the reader lock in `finally`. If the response has no body, throw `Error("Notification stream has no response body")`.

- [ ] **Step 5: Run tests and frontend type checking**

```bash
mise run frontend:test -- src/lib/api/client.test.ts src/lib/api/sse.test.ts
mise run frontend:check
```

Expected: PASS.

- [ ] **Step 6: Commit transport primitives**

```bash
git add frontend/src/lib/api/types.ts frontend/src/lib/api/client.ts frontend/src/lib/api/client.test.ts frontend/src/lib/api/sse.ts frontend/src/lib/api/sse.test.ts
git commit -m "feat(frontend): add notification stream transport"
```

### Task 10: Session-Scoped Notification Store And Account Reconciliation

**Files:**
- Create: `frontend/src/lib/stores/notifications.svelte.ts`
- Create: `frontend/src/lib/stores/notifications.svelte.test.ts`
- Modify: `frontend/src/lib/stores/accounts.svelte.ts:21-46`
- Modify: `frontend/src/lib/stores/accounts.svelte.test.ts`

**Interfaces:**
- Consumes Task 9 API and SSE helpers plus global `auth` and `accounts` stores.
- Produces:

```ts
export type ReconcileReason =
  | "initial"
  | "connected"
  | "live"
  | "visibility"
  | "manual"
  | "recovery";

export interface NotificationToast {
  id: string;
  notification: Notification;
}

class NotificationsStore {
  items: Notification[];
  unreadCount: number;
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  loadingMore: boolean;
  loadMoreError: string | null;
  nextCursor: string | null;
  toasts: NotificationToast[];
  get recent(): Notification[];
  get hasMore(): boolean;
  start(): void;
  reset(): void;
  reconcile(reason?: ReconcileReason): Promise<void>;
  loadMore(): Promise<void>;
  markRead(id: string): Promise<void>;
  markAllRead(): Promise<void>;
  dismissToast(id: string): void;
  activityVersion(accountId: string): number;
}

export const notifications: NotificationsStore;
```

- Extends `accounts.load(signal?: AbortSignal): Promise<boolean>`, where `true` means the newest response was applied and `false` means failure, cancellation, reset, or supersession. Existing callers may ignore the result.

- [ ] **Step 1: Write failing account refresh preservation tests**

Add tests that load a successful account list, start a refresh, fail it with 503, and assert the previous items remain. Add a test that aborts a load and asserts no user-visible account error is set. Keep existing reset and load-sequence assertions.

- [ ] **Step 2: Make account loading abortable and non-destructive**

Pass `signal` through the existing request options. On refresh, do not clear `items` or `loaded`. Return `true` only after assigning the newest response. Treat `signal.aborted` as cancellation rather than an error and return `false`; also return `false` for a failed, reset, or superseded load. Preserve generation and load-sequence checks exactly.

Run:

```bash
mise run frontend:test -- src/lib/stores/accounts.svelte.test.ts
```

Expected: PASS.

- [ ] **Step 3: Write failing notification store tests**

Cover these exact behaviors with deferred fetches and fake timers:

```ts
it("loads history and unread count without replaying toasts")
it("coalesces burst invalidations into one reconciliation")
it("refreshes accounts before publishing a live toast")
it("reconnects with bounded backoff and resets delay after connection")
it("reconciles visibility recovery without historical toasts")
it("aborts stream and ignores stale results after reset")
it("merges cursor pages without duplicate notification ids")
it("rolls back mark-read state and reconciles after failure")
it("uses mutation responses as the authoritative unread count")
it("increments activity only for affected account ids")
```

Mock `request`, `requestResponse`, `consumeEventStream`, and `accounts.load` at module boundaries. Assert `accounts.load` resolves before the toast appears.

- [ ] **Step 4: Implement session lifecycle and authoritative reconciliation**

Use `$state.raw` for API response arrays that are replaced rather than mutated. Keep private non-reactive fields for generation, abort controller, known IDs, reconnect timer, in-flight reconciliation, and queued reason.

`start()` captures `auth.generation`, creates one session `AbortController`, performs `reconcile("initial")`, installs `visibilitychange`, and starts one stream loop. Calling it again for the same generation is a no-op.

Stream loop:

```ts
const response = await requestResponse("/notifications/stream", {
  authenticated: true,
  signal,
});
await reconcile("connected");
await consumeEventStream(response, () => queueReconcile("live"), signal);
```

Use reconnect delays `500, 1000, 2000, 5000, 10000, 30000` milliseconds and cap at 30 seconds. Reset the index after a stream opens. Before every state write, require both the captured store generation and `auth.generation` to match.

- [ ] **Step 5: Implement reconciliation, pagination, reads, and toast expiry**

Rules:

- Fetch `/notifications?size=20` for first-page reconciliation.
- Preserve already loaded older pages while replacing matching first-page IDs.
- Set the count directly from `unread_count`.
- Put IDs first discovered during a live reconciliation into a private pending-live map, then await `accounts.load(signal)`. Publish and remove those pending toasts only when account loading returns `true`; a later successful reconciliation may release pending live toasts, but initial/reconnect/visibility rows that were never discovered live never enter this map.
- Add toasts only for IDs absent from `knownIds` when reason is `live`.
- Increment per-account activity versions for newly discovered rows after any non-initial recovery; live and reconnect recovery therefore refresh open history.
- Coalesce concurrent invalidations into one active reconciliation plus at most one queued rerun.
- Load more from `/notifications?size=20&cursor=${encodeURIComponent(nextCursor)}` and deduplicate by ID.
- Optimistically mark rows/read count; call `PUT /notifications/:id/read` or `PUT /notifications/read-all`; replace count from the response; on failure restore row state and run `reconcile("recovery")`.
- Auto-dismiss each toast after 5 seconds; `reset()` clears timers.

- [ ] **Step 6: Run store tests and autofix the Svelte module**

```bash
mise run frontend:test -- src/lib/stores/accounts.svelte.test.ts src/lib/stores/notifications.svelte.test.ts
```

Run the Svelte autofixer on `frontend/src/lib/stores/notifications.svelte.ts`; apply fixes and rerun until clean. Then run:

```bash
mise run frontend:check
```

Expected: all tests and checks PASS.

- [ ] **Step 7: Commit session-scoped state**

```bash
git add frontend/src/lib/stores/accounts.svelte.ts frontend/src/lib/stores/accounts.svelte.test.ts frontend/src/lib/stores/notifications.svelte.ts frontend/src/lib/stores/notifications.svelte.test.ts
git commit -m "feat(frontend): reconcile live notification state"
```

### Task 11: Notification Bell, Shared Rows, And Live Toasts

**Files:**
- Create: `frontend/src/lib/components/NotificationItem.svelte`
- Create: `frontend/src/lib/components/NotificationItem.test.ts`
- Create: `frontend/src/lib/components/NotificationBell.svelte`
- Create: `frontend/src/lib/components/NotificationBell.test.ts`
- Create: `frontend/src/lib/components/NotificationToasts.svelte`
- Create: `frontend/src/lib/components/NotificationToasts.test.ts`
- Modify: `frontend/src/lib/components/AppHeader.svelte`
- Modify: `frontend/src/lib/components/AppHeader.test.ts`

**Interfaces:**
- Shared item props:

```ts
interface Props {
  notification: Notification;
  compact?: boolean;
  disabled?: boolean;
  onactivate: (notification: Notification) => void | Promise<void>;
}
```

- Bell and toast components consume the singleton `notifications` store directly.

- [ ] **Step 1: Write failing shared item and toast tests**

Assert sent rows show a negative formatted amount with semantic error styling, received rows show a positive amount with success styling, unread rows include visible text `Unread`, and activation calls the supplied callback once.

For toasts, seed store toasts, render the component, and assert a persistent `aria-live="polite"` container, correct sent/received text, no focus movement, keyed removal, and timer-based expiry.

- [ ] **Step 2: Implement `NotificationItem` and `NotificationToasts`**

Use daisyUI `list`/`list-row` structure and existing `formatSignedMoney`. The row must be a real button with `min-h-11`, an accessible label containing direction, amount, currency, and time, and unread status conveyed by text/weight rather than color alone.

Use a persistent:

```svelte
<div class="toast toast-top toast-end" aria-live="polite" aria-atomic="false">
```

Render each toast with daisyUI `alert` styling but no `role="alert"`; the persistent polite parent owns announcement priority. Do not move focus. Timer ownership remains in the store.

- [ ] **Step 3: Write failing bell interaction tests**

Mock the notification store and router. Assert:

- Accessible name is `Notifications, 124 unread` while visual badge is `99+` and `aria-hidden`.
- Opening does not call a read method.
- Clicking an unread row awaits `markRead`, then closes popover and navigates.
- Failed `markRead` leaves popover open, restores state through the store, and shows a compact retryable error.
- Clicking a read row navigates without a write.
- `Mark all read`, retry, and `View all notifications` work by keyboard.

- [ ] **Step 4: Implement native popover bell**

Use the daisyUI Popover API structure from the repository skill:

```svelte
<button popovertarget="notification-preview" style="anchor-name:--notification-bell">
  <!-- Bell icon and visual badge -->
</button>
<section
  id="notification-preview"
  class="dropdown dropdown-end"
  popover
  style="position-anchor:--notification-bell"
>
```

Bind the popover element only to call `hidePopover()` after a successful read or immediate read-row navigation. Keep it open on failure. Place the bell before `ThemeToggle` in `AppHeader`. At 320px, hide nonessential badge text if needed but preserve the button's complete accessible name and 44px target.

- [ ] **Step 5: Run component tests and Svelte autofixer**

```bash
mise run frontend:test -- src/lib/components/NotificationItem.test.ts src/lib/components/NotificationBell.test.ts src/lib/components/NotificationToasts.test.ts src/lib/components/AppHeader.test.ts
```

Run Svelte autofixer on all four changed/created `.svelte` files until clean, then:

```bash
mise run frontend:check
```

Expected: PASS.

- [ ] **Step 6: Commit the header notification UI**

```bash
git add frontend/src/lib/components/NotificationItem.svelte frontend/src/lib/components/NotificationItem.test.ts frontend/src/lib/components/NotificationBell.svelte frontend/src/lib/components/NotificationBell.test.ts frontend/src/lib/components/NotificationToasts.svelte frontend/src/lib/components/NotificationToasts.test.ts frontend/src/lib/components/AppHeader.svelte frontend/src/lib/components/AppHeader.test.ts
git commit -m "feat(frontend): add notification bell and live toasts"
```

### Task 12: Full History Route, App Lifecycle, And Activity Refresh

**Files:**
- Create: `frontend/src/lib/pages/NotificationsPage.svelte`
- Create: `frontend/src/lib/pages/NotificationsPage.test.ts`
- Modify: `frontend/src/App.svelte`
- Modify: `frontend/src/App.test.ts`
- Modify: `frontend/src/lib/pages/AccountHistoryPage.svelte`
- Modify: `frontend/src/lib/pages/AccountHistoryPage.test.ts`

**Interfaces:**
- Consumes the singleton notification store, shared `NotificationItem`, and `NotificationToasts`.
- Produces protected route `/notifications` with title/announcement label `Notifications`.
- Account activity reads `notifications.activityVersion(accountId)` to trigger retained-data refreshes.

- [ ] **Step 1: Write failing notification page tests**

Mock the store and assert:

```ts
it("renders loading, empty, and retained-data error states")
it("marks an unread item before navigating to its account")
it("keeps the page visible when mark-read fails")
it("loads the next cursor page without duplicate rows")
it("marks all notifications read")
```

The failure test must assert no navigation occurs until `markRead` resolves successfully.

- [ ] **Step 2: Implement the full history page**

Render a responsive max-width page with heading `Notifications`, `Mark all read`, keyed shared rows, and a `Load more` button. Distinguish initial loading from refresh so existing rows stay visible. Show full-page retry for initial failure and inline retry for refresh/pagination failures. Disable conflicting read mutations while one is pending.

- [ ] **Step 3: Write failing App lifecycle and route tests**

Extend the hoisted mocks with a notification store. Assert:

- Authenticated resolution calls `notifications.start()` once.
- Signed-out resolution calls `notifications.reset()` and `accounts.reset()`.
- `/notifications` renders the page, updates title to `Notifications · SimpleBank`, announces navigation, and remains auth guarded.
- Authenticated chrome contains the persistent toast live region.
- Component unmount resets/aborts notification resources.

- [ ] **Step 4: Wire App lifecycle, route, and toasts**

Add `NotificationsPage` to the protected switch and `NotificationToasts` inside authenticated chrome. In the auth effect:

```ts
if (!auth.initializing && auth.isAuthenticated) {
  notifications.start();
} else if (!auth.initializing) {
  notifications.reset();
  accounts.reset();
}
```

Return teardown or use `onDestroy` so root unmount calls `notifications.reset()`. Do not restart for access-token refresh inside the same auth generation.

- [ ] **Step 5: Write failing account-history live refresh tests**

Add tests proving:

- Changing `activityVersion(accountA)` reloads account A and transfers.
- Existing account and transfer rows stay visible while refresh is pending.
- Refresh failure displays compact retry feedback without clearing successful data.
- A stale refresh response after auth generation changes is ignored.
- Route change still clears account A before showing account B.

- [ ] **Step 6: Refactor account history loading with cancellation and preservation**

The effect reads route account ID, activity version, and auth generation. Give each effect run an `AbortController` and return `abort`. Route changes clear old content; same-account activity refresh passes `preserveVisibleData = true`. Every apply path checks local load generation and auth generation. Pass `signal` into both API requests.

Do not make the notification store own page-local transfer arrays; it only supplies the reactive invalidation version.

- [ ] **Step 7: Run all route/page tests and Svelte autofixer**

```bash
mise run frontend:test -- src/lib/pages/NotificationsPage.test.ts src/lib/pages/AccountHistoryPage.test.ts src/App.test.ts
```

Run Svelte autofixer on `NotificationsPage.svelte`, `AccountHistoryPage.svelte`, and `App.svelte` until clean. Then run:

```bash
mise run frontend:check
mise run frontend:lint
mise run frontend:format:check
```

Expected: PASS.

- [ ] **Step 8: Commit page and lifecycle integration**

```bash
git add frontend/src/lib/pages/NotificationsPage.svelte frontend/src/lib/pages/NotificationsPage.test.ts frontend/src/lib/pages/AccountHistoryPage.svelte frontend/src/lib/pages/AccountHistoryPage.test.ts frontend/src/App.svelte frontend/src/App.test.ts
git commit -m "feat(frontend): add durable notification history"
```

### Task 13: Browser Flows, Accessibility, And End-To-End Verification

**Files:**
- Create: `frontend/e2e/support/mock-api.ts`
- Create: `frontend/e2e/notifications.spec.ts`
- Modify: `frontend/e2e/accessibility.spec.ts`
- Modify only if runtime proof finds a defect: files from Tasks 1-12

**Interfaces:**
- Produces reusable controlled browser mock:

```ts
export interface AuthenticatedApiMock {
  setAccounts(accounts: Account[]): void;
  setNotifications(page: NotificationPage): void;
  emitNotification(id: string): Promise<void>;
  closeStream(): Promise<void>;
}

export function mockAuthenticatedAPI(
  page: Page,
  accounts?: Account[],
): Promise<AuthenticatedApiMock>;

export function expectNoAccessibilityViolations(page: Page): Promise<void>;
```

- [ ] **Step 1: Extract existing Playwright API and Axe helpers**

Move `user`, account fixtures, `mockAuthenticatedAPI`, and `expectNoAccessibilityViolations` from `accessibility.spec.ts` into `e2e/support/mock-api.ts`. Preserve existing route registration order. Add default successful mocks for notification history, mark-one, and mark-all so existing tests report no failed requests.

- [ ] **Step 2: Add a controlled fetch-based SSE mock**

Before navigation, install a browser-side wrapper with `page.addInitScript`. For `/api/v1/notifications/stream`, return a `Response` whose `ReadableStream` controller is retained on `window`; delegate every other request to the original fetch. `emitNotification(id)` enqueues:

```text
event: notification
data: <id>

```

`closeStream()` closes the current controller and allows the next stream request to create a new controller. Keep all mock control methods in Node by calling `page.evaluate`.

- [ ] **Step 3: Write sender and recipient live-update browser tests**

Create scenarios that:

1. Open dashboard with initial balance.
2. Change mocked accounts and notification first page.
3. Emit the notification ID.
4. Assert balance and unread badge update in under one second.
5. Assert the correctly signed toast appears once.
6. Open the bell, activate the row, verify read request finishes, badge changes, and account history route opens.

Use one sent and one received notification. Add a burst test that emits two IDs before reconciliation completes and asserts one authoritative notification/history fetch and no duplicate toast IDs.

- [ ] **Step 4: Write recovery and full-history browser tests**

Test stream close/reconnect: close the stream, update durable history, wait for reconnect reconciliation, assert balances/history update, and assert no recovery toast. Test `/notifications` pagination, mark-all authoritative count, retained-data error, keyboard activation, and back navigation.

- [ ] **Step 5: Extend responsive and accessibility coverage**

Include the notification bell in the existing 320px non-overlap controls. Run Axe on the bell popover and full history in both themes at 320px and 1440px. Assert no horizontal overflow, every interactive target is at least 44px, keyboard opening/closing preserves logical focus, and live toasts do not steal focus.

- [ ] **Step 6: Run focused and complete frontend gates**

```bash
mise run frontend:test:e2e -- e2e/notifications.spec.ts
mise run frontend:check
mise run frontend:lint
mise run frontend:format:check
mise run frontend:test
mise run frontend:test:e2e
```

If Chromium is not installed, run `mise run frontend:test:e2e:install` once, then rerun the failed command. Expected: all PASS.

- [ ] **Step 7: Run complete backend, database, security, and build gates**

```bash
mise run sqlc:generate
git diff --exit-code -- internal/db/sqlc
mise run golangci-lint:fmt
mise run golangci-lint
mise run test:unit
mise run test:integration
mise run govulncheck
mise run app:build
```

Expected: all PASS and generated code remains unchanged after regeneration.

- [ ] **Step 8: Exercise the real runtime path**

Start PostgreSQL with `mise run compose:test:up`. Run the application with valid `DB_SOURCE`, a `JWT_SECRET` of at least 32 characters, and `SMTP_FROM` using `mise run app -- serve`. With two authenticated users:

1. Open each authenticated notification stream.
2. Submit one transfer.
3. Confirm both streams receive their distinct notification IDs in under one second.
4. Confirm REST history returns sender/recipient directions and balances.
5. Disconnect one stream, submit another transfer, reconnect, and confirm REST reconciliation recovers the missed durable row without replaying it as a live toast.
6. Stop the process and confirm logs show HTTP, River, listener, then pool shutdown order with no hanging stream.

Always stop PostgreSQL with `mise run compose:test:down`.

- [ ] **Step 9: Commit browser coverage and any verified fixes**

```bash
git add frontend/e2e/support/mock-api.ts frontend/e2e/notifications.spec.ts frontend/e2e/accessibility.spec.ts
git commit -m "test: cover real-time balance notifications"
```

If runtime proof required production fixes, stage each intended path explicitly in the first command. Do not use `git add -u` or stage unrelated worktree changes. Before committing, inspect `git status --short` and `git diff --cached`.
