-- name: InitializeLoginThrottlePair :exec
INSERT INTO login_throttles (
    scope,
    key_hash,
    attempt_count,
    window_started_at,
    blocked_until,
    expires_at
)
VALUES (
    sqlc.arg(account_scope),
    sqlc.arg(account_key_hash),
    0,
    sqlc.arg(now)::timestamptz,
    NULL,
    sqlc.arg(now)::timestamptz + make_interval(secs => sqlc.arg(retention_seconds)::integer)
), (
    sqlc.arg(client_scope),
    sqlc.arg(client_key_hash),
    0,
    sqlc.arg(now)::timestamptz,
    NULL,
    sqlc.arg(now)::timestamptz + make_interval(secs => sqlc.arg(retention_seconds)::integer)
)
ON CONFLICT (scope, key_hash) DO NOTHING;

-- name: GetLoginThrottlePairForUpdate :many
SELECT *
FROM login_throttles
WHERE (scope = sqlc.arg(account_scope) AND key_hash = sqlc.arg(account_key_hash))
   OR (scope = sqlc.arg(client_scope) AND key_hash = sqlc.arg(client_key_hash))
ORDER BY scope, key_hash
FOR UPDATE;

-- name: AdvanceLoginThrottleAttempt :one
UPDATE login_throttles
SET attempt_count = CASE
        WHEN login_throttles.window_started_at <
             sqlc.arg(now)::timestamptz - make_interval(secs => sqlc.arg(window_seconds)::integer)
            THEN 1
        ELSE login_throttles.attempt_count + 1
    END,
    window_started_at = sqlc.arg(now)::timestamptz,
    blocked_until = CASE
        WHEN login_throttles.window_started_at <
             sqlc.arg(now)::timestamptz - make_interval(secs => sqlc.arg(window_seconds)::integer)
            THEN NULL
        ELSE login_throttles.blocked_until
    END,
    expires_at = sqlc.arg(now)::timestamptz +
        make_interval(secs => sqlc.arg(retention_seconds)::integer)
WHERE scope = sqlc.arg(scope)
  AND key_hash = sqlc.arg(key_hash)
RETURNING *;

-- name: SetLoginThrottleBlockedUntil :one
UPDATE login_throttles
SET blocked_until = GREATEST(
        COALESCE(blocked_until, sqlc.arg(blocked_until)::timestamptz),
        sqlc.arg(blocked_until)::timestamptz
    ),
    expires_at = GREATEST(expires_at, sqlc.arg(blocked_until)::timestamptz)
WHERE scope = sqlc.arg(scope)
  AND key_hash = sqlc.arg(key_hash)
RETURNING *;

-- name: DeleteLoginThrottle :exec
DELETE FROM login_throttles
WHERE scope = $1 AND key_hash = $2;

-- name: DeleteExpiredLoginThrottles :execrows
DELETE FROM login_throttles
WHERE expires_at <= $1;
