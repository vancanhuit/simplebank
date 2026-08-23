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
