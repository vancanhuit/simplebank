# Real-Time Balance Notifications Final Fix Report

Date: 2026-08-23

## Scope

This wave addresses the complete final-review finding set without changing the notification store public API, database isolation level, lifecycle order, or transfer invariants.

## Important 1: Frontend Mutation/List Race

### Finding

A first-page reconciliation or pagination response started before a successful read mutation could complete afterward and overwrite authoritative `read_at` and `unread_count` state.

### Change

- Added a session-scoped, non-reactive mutation epoch to `NotificationsStore`.
- First-page reconciliation and `loadMore` capture the epoch before making their request.
- Successful `markRead` and `markAllRead` advance the epoch before applying their authoritative mutation response.
- A list response crossing the epoch applies no notification, count, cursor, baseline, activity, or toast state and queues a fresh `recovery` reconciliation.
- Reset clears the epoch. Existing mutation serialization, auth/session generation checks, toast baseline, pagination dedupe, and exported API remain unchanged.

### TDD Evidence

RED:

```text
mise run frontend:test -- src/lib/stores/notifications.svelte.test.ts
FAIL discards a first-page response that crosses a successful mark-read
expected request to be called 4 times, got 3
FAIL discards a pagination response that crosses a successful mark-all
expected request to be called 4 times, got 3 (focused run)
```

GREEN:

```text
mise run frontend:test -- src/lib/stores/notifications.svelte.test.ts -t 'discards a first-page response|discards a pagination response'
2 passed, 15 skipped

mise run frontend:test -- src/lib/stores/notifications.svelte.test.ts
17 passed
```

### Files

- `frontend/src/lib/stores/notifications.svelte.ts`
- `frontend/src/lib/stores/notifications.svelte.test.ts`

### Svelte Guidance And Autofixer

- Consulted official Svelte sections `svelte/svelte-js-files`, `svelte/$state`, and `svelte/testing`.
- Ran `svelte_svelte-autofixer` against `notifications.svelte.ts` targeting Svelte 5.
- Result: no issues, no suggestions, no follow-up required.

## Important 2: Cancellable Service Startup

### Finding

`runServices` passed `context.WithoutCancel(ctx)` to listener and worker startup, allowing blocked startup to ignore process termination.

### Change

- `listener.Start(ctx)` and `worker.Start(ctx)` now receive the cancellable application context.
- `context.WithoutCancel(ctx)` remains only as the parent for independent ten-second shutdown contexts.
- Existing listener -> worker -> HTTP startup, HTTP -> worker -> listener shutdown, and joined errors remain intact.
- Lifecycle tests now prove a canceled listener startup prevents later startup, and cancellation observed between listener and worker startup prevents HTTP while still stopping the listener with an independent bounded context.

### TDD Evidence

RED:

```text
go test -race ./cmd/app -run '^TestRunServices'
FAIL TestRunServicesListenerStartFailurePreventsWorkerAndHTTP: got nil, want context canceled
FAIL TestRunServicesCanceledWorkerStartupPreventsHTTP: got nil, want context canceled
```

GREEN:

```text
go test -race ./cmd/app -run '^TestRunServices'
ok github.com/vancanhuit/simplebank/cmd/app
```

### Files

- `cmd/app/main.go`
- `cmd/app/main_test.go`

## Minor 1: LISTEN Cleanup

### Change

`TestNotificationPublishIsCommitGated` now executes `UNLISTEN balance_notifications` with a bounded cleanup context before releasing its pooled connection.

### Evidence

The full integration suite passed, including `TestNotificationPublishIsCommitGated`.

### File

- `internal/db/notifications_query_test.go`

## Minor 2: Concurrent Same-Key Publish Count

### Change

- Extended `TestTransferTxConcurrentSameKey` to LISTEN on a dedicated `pgx.Conn`, avoiding shared pool capacity and session state.
- It reads exactly two committed messages, verifies one owner per side, then uses a bounded wait to prove no third message exists.
- Cleanup performs `UNLISTEN` and closes the dedicated connection.

### TDD / Iteration Evidence

The first run using a pooled LISTEN connection timed out waiting for blocked workers because it reduced pool capacity. The test was corrected to use a dedicated connection.

GREEN:

```text
go test -race -tags=integration ./internal/db -run '^TestTransferTxConcurrentSameKey$'
ok github.com/vancanhuit/simplebank/internal/db
```

### File

- `internal/db/transfer_safety_test.go`

## Minor 3: Repeatable-Read Snapshot Regression

### Change

- Added `TestListNotificationsPageUsesOneRepeatableReadSnapshot`.
- Added one unexported test callback on `SQLStore`, invoked after list and before count.
- The callback commits a second notification from another connection exactly between the two reads.
- The page returns one row and unread count one, while a post-transaction query sees two, directly proving one repeatable-read snapshot.
- Production remains `pgx.RepeatableRead`; the callback is nil in normal construction.

### TDD Evidence

RED:

```text
go test -race -tags=integration ./internal/db -run '^TestListNotificationsPageUsesOneRepeatableReadSnapshot$'
build failed: snapshotStore.afterListNotifications undefined
```

GREEN:

```text
go test -race -tags=integration ./internal/db -run '^TestListNotificationsPageUsesOneRepeatableReadSnapshot$'
ok github.com/vancanhuit/simplebank/internal/db
```

### Files

- `internal/db/store.go`
- `internal/db/notification_tx.go`
- `internal/db/notifications_query_test.go`

## Minor 4: SSE Write-Deadline Failure Cleanup

### Change

- Added an unexported subscription function on `Server`, initialized to `notificationHub.Subscribe`.
- Added a focused response writer implementing `SetWriteDeadline` and forcing an error before the first frame.
- The regression proves the request returns 500, no connected SSE frame is written, and deferred unsubscribe runs.

### TDD Evidence

RED:

```text
go test -race ./internal/api -run '^TestNotificationStreamWriteDeadlineFailureUnsubscribes$'
build failed: s.subscribeNotifications undefined
```

GREEN:

```text
go test -race ./internal/api -run '^TestNotificationStreamWriteDeadlineFailureUnsubscribes$'
ok github.com/vancanhuit/simplebank/internal/api
```

### Files

- `internal/api/server.go`
- `internal/api/notification.go`
- `internal/api/notification_test.go`

## Full Verification

```text
mise run frontend:check
svelte-check found 0 errors and 0 warnings

mise run frontend:lint
PASS

mise run frontend:format:check
All matched files use Prettier code style

mise run frontend:test
28 files passed, 189 tests passed

mise run frontend:test:e2e
22 passed

mise run golangci-lint:fmt
PASS

mise run golangci-lint
0 issues

mise run test:unit
PASS

mise run test:integration
PASS; internal/db 80.2% coverage; compose:test:down completed and removed containers, volumes, and network

mise run app:build
PASS (sequential rerun)
```

The first `app:build` attempt ran in parallel with another frontend build and observed `frontend/dist` while it was being replaced, causing embed-file-not-found errors. A sequential rerun completed successfully.

## Concerns

- No unresolved correctness concerns.
- The two production test seams are unexported and nil/defaulted in production paths; they add no public API or configuration.
- No SQL, migration, or generated sqlc files changed.
