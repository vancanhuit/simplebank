# Real-Time Balance Notifications Design

## Status

Approved on 2026-08-23.

## Goal

Notify both owners affected by every successful transfer, update their visible
account balances in under one second while connected, and retain notifications
so changes made while a user is offline appear after the next sign-in.

Notifications have read/unread state. The header shows an unread badge and a
preview, live changes produce a transient toast, and a dedicated page provides
the complete history. Clicking a notification marks it read and opens the
affected account's activity page. Users can also mark all notifications read.

## Existing Invariants

- Established account balances change only in `TransferTx`. Account creation may
  set an opening balance, but it does not change a balance previously visible to
  the user and therefore does not create a balance-change notification.
- Transfers preserve source-first authorization, source-scoped idempotency,
  deterministic account locking, validation on locked rows, rolling daily
  limits, guarded balance updates, and ledger entries.
- Durable notification rows must commit or roll back with the balance changes.
- A repeated or concurrent idempotent transfer must not create duplicate
  notifications.
- Backend lifecycle remains migrations -> notification listener -> River -> HTTP
  at startup and HTTP -> River -> notification listener -> database pool at
  shutdown.
- Frontend data remains session-scoped. Requests and events started before
  logout must not update a later session.
- The existing `UNIQUE (owner, currency)` account invariant remains unchanged.
  Transfers require matching currencies, so a user cannot transfer between two
  accounts they own under the current account model.

## Data Model

Add a `notifications` table with:

- `id`: UUID primary key.
- `owner`: the username authorized to read the notification.
- `account_id`: the affected account.
- `transfer_id`: the transfer that changed the balance.
- `direction`: `sent` or `received`.
- `amount`: the positive transfer amount in minor units. Direction determines
  presentation sign.
- `currency`: the account currency at the time of the transfer.
- `balance`: the affected account's post-transfer balance in minor units.
- `read_at`: nullable timestamp; null means unread.
- `created_at`: notification creation timestamp.

Enforce one notification per affected account and transfer with a unique
constraint on `(transfer_id, account_id)`. Index `(owner, created_at DESC, id
DESC)` for history, plus an owner-scoped partial index for unread rows. Foreign
keys reference the transfer and account records.

Create two rows inside `TransferTx`, after the balance updates and transfer row
exist but before transaction completion. One row describes the sender account
and one describes the recipient account. Notification identity remains scoped
to the affected account even though the current account uniqueness invariant
means sender and recipient owners are distinct.

The transaction also calls PostgreSQL `pg_notify` once for each row with a small
internal payload containing its notification ID and owner. The owner lets every
replica route the invalidation without an extra database lookup; only the ID is
forwarded to the browser. PostgreSQL delivers the messages only after commit, so
rolled-back transfers cannot generate live UI changes. Durable rows, not
`NOTIFY`, remain the source of truth.

Notification SQL belongs in `internal/db/query`, generated access in
`internal/db/sqlc`, and transfer orchestration in handwritten transaction code.
The existing wide `db.Store` remains the API database boundary.

## Real-Time Delivery

The application composition root owns a PostgreSQL notification listener and an
in-process subscriber hub. Every application replica listens on the same
database channel. Therefore, a transfer handled by one replica can reach a
browser whose stream is connected to another replica without sticky sessions.

The listener starts after database migrations and before River and HTTP. It uses
a dedicated PostgreSQL connection, reconnects with bounded exponential backoff,
and republishes received notification IDs to locally connected subscribers. A
slow or disconnected subscriber cannot block database listening or other users;
the hub uses bounded delivery and may drop an invalidation event because clients
reconcile from durable state.

The listener stops after HTTP and River have stopped, and before the shared pool
closes. Readiness continues to mean that the database is reachable. A temporary
listener failure degrades latency but not correctness because durable history
and reconnect reconciliation recover every committed change.

## API And Authorization

Add authenticated endpoints to:

- List the current user's notifications with keyset/cursor pagination, newest
  first. Its response envelope includes the authoritative unread count and next
  cursor, so history and badge state come from one snapshot.
- Mark one current-user notification read.
- Mark all current-user notifications read.
- Stream notification invalidations with Server-Sent Events.

Read mutation responses return the resulting authoritative unread count. This
avoids client-side count drift when a new notification races a read operation.

All list, count, and update SQL includes the authenticated username. A caller
cannot read or mutate another user's notification by guessing its UUID. The SSE
handler subscribes only to events owned by the authenticated user.

The frontend opens SSE with `fetch`, attaching the existing bearer access token,
then parses the standard event stream. This avoids credentials in URLs, avoids a
second ticket authentication mechanism, and works across replicas. The server
sends only notification IDs as invalidation signals rather than treating stream
payloads as authoritative account or notification data. It emits periodic SSE
comments as keepalives, sets the stream lifetime no later than the authenticated
access token's expiry, and removes subscriptions promptly when request contexts
end.

An access-token expiry or connection failure closes the stream. The frontend
uses the existing refresh flow when needed, then reconnects with bounded backoff
while the authenticated session is current. Every successful connection or
reconnection triggers authoritative reconciliation, recovering offline changes,
dropped invalidations, and events committed during connection setup.

## Frontend State And Flow

Add a session-scoped notification store. After authentication it:

1. Loads the first notification history page and unread count.
2. Opens the authenticated SSE stream.
3. Reconciles notifications and accounts after each invalidation.
4. Reconciles after reconnect and when a hidden tab becomes visible.
5. Aborts the stream, pending requests, and reconnect timers on logout, then
   clears all notification state.

Every asynchronous result is guarded by the auth/session generation. Account
updates continue through the account store so its generation and load-sequence
checks prevent stale responses from repopulating a new session.

The event is an invalidation, not a state delta. On receipt, the client fetches
authoritative notification data and calls `accounts.load()`. Coalesce concurrent
invalidations into one reconciliation so bursts do not trigger one full refresh
per event. If the user is viewing the affected account activity page, refresh
its transfer history as part of that page's event response.

Only notifications first discovered from a live invalidation produce a toast.
Initial load, reconnect reconciliation, visibility restoration, and pagination
must not replay historical notifications as live toasts. A live notification
updates the unread badge and account balances before its polite live-region
toast is announced.

## User Interface

Use the approved hybrid layout:

- Add an accessible bell button to the authenticated header with an unread-count
  badge. The badge has a concise accessible label and caps its visual text when
  counts are large without changing the actual count available to assistive
  technology.
- The bell opens a keyboard-operable popover containing recent notifications,
  a `Mark all read` action, and a `View all notifications` link. Opening the
  popover does not mark items read.
- Clicking an unread notification marks only that item read and updates the
  badge optimistically. Navigation to `/accounts/:accountId` follows after the
  write succeeds and its authoritative unread count is applied. On failure, the
  optimistic state rolls back, the popover stays open, and a compact retryable
  error is shown. Already-read items navigate without another write.
- A live balance change produces a brief toast describing sent or received
  money. Toasts use a persistent polite live region, do not steal focus, and
  expire automatically.
- Add `/notifications` as a protected route. It shows cursor-paginated history,
  read/unread styling, empty/loading/error states, retry controls, individual
  notification navigation, and `Mark all read`.
- Preserve the existing daisyUI visual language, semantic colors, minimum touch
  targets, responsive header behavior, and light/dark themes.

## Error Handling

- Keep the last successfully loaded balances and notifications visible during a
  transient refresh failure.
- Reconnect SSE with bounded exponential backoff and reset the delay after a
  healthy connection. Do not show repeated connection-error toasts.
- Surface history loading and mutation failures on the full notifications page
  with retry controls. In the header preview, preserve existing data and expose
  a compact retry state.
- Replace optimistic unread-count state with each successful mutation response.
  If an individual or bulk read request fails, roll back local read state and
  reconcile to resolve any concurrent live notification.
- Treat malformed SSE frames or unknown notification IDs as invalidations that
  require reconciliation, not as trusted data.

## Testing

### Database Integration

- A successful transfer creates exactly one sender and one recipient
  notification with correct owner, account, direction, amount, currency, and
  post-transfer balance.
- Failed transactions create neither balance changes nor notifications.
- Idempotent replay and concurrent duplicate requests do not create duplicate
  notifications.
- History, unread count, mark-one-read, and mark-all-read are correctly
  owner-scoped and cursor-paginated.
- PostgreSQL notifications are visible only after transaction commit.

### API

- Notification endpoints require authentication and isolate owners.
- Pagination is stable for equal timestamps by using `(created_at, id)` cursors.
- Individual and bulk read operations are idempotent.
- SSE rejects unauthenticated requests, filters by owner, emits keepalives and
  invalidations, and cleans up disconnected subscribers.
- Listener/hub tests cover reconnect, bounded slow-subscriber behavior, and
  cross-replica delivery through PostgreSQL.

### Frontend

- Notification store initial load, event coalescing, reconnect reconciliation,
  visibility reconciliation, and logout/reset races.
- Initial and recovered notifications do not create duplicate toasts; newly
  discovered live notifications do.
- Live events refresh account balances and affected account history.
- Bell badge, popover, mark-one, mark-all, navigation, pagination, loading,
  empty, and failure behavior.
- Keyboard interaction, focus behavior, live-region announcements, accessible
  names, contrast, and mobile layout.
- Playwright covers sender and recipient balance updates and notifications using
  controlled mocked SSE streams.

## Verification

Run sqlc generation after schema/query changes, then the database integration
suite. Run backend formatting, lint, unit tests, and vulnerability checks for the
new authenticated streaming surface. Run frontend check, lint, format check,
unit tests, and Playwright. Finally build the complete application and exercise
startup, an authenticated stream, transfer delivery, reconnect recovery, and
ordered shutdown against PostgreSQL.

## Out Of Scope

- Email, SMS, browser push, or operating-system notifications.
- Notification types unrelated to account balance changes.
- User-configurable notification preferences or retention periods.
- Deleting or archiving notifications.
- WebSockets or general bidirectional real-time messaging.
