-- name: GetLoginThrottle :one
SELECT *
FROM login_throttles
WHERE scope = $1 AND key_hash = $2
LIMIT 1;

-- name: IncrementLoginThrottle :one
INSERT INTO login_throttles (
    scope,
    key_hash,
    failure_count,
    window_started_at,
    blocked_until,
    expires_at
)
VALUES (
    sqlc.arg(scope),
    sqlc.arg(key_hash),
    1,
    sqlc.arg(now)::timestamptz,
    NULL,
    sqlc.arg(now)::timestamptz + make_interval(secs => sqlc.arg(retention_seconds)::integer)
)
ON CONFLICT (scope, key_hash) DO UPDATE
SET failure_count = CASE
        WHEN login_throttles.window_started_at <=
             sqlc.arg(now)::timestamptz - make_interval(secs => sqlc.arg(window_seconds)::integer)
            THEN 1
        ELSE login_throttles.failure_count + 1
    END,
    window_started_at = CASE
        WHEN login_throttles.window_started_at <=
             sqlc.arg(now)::timestamptz - make_interval(secs => sqlc.arg(window_seconds)::integer)
            THEN sqlc.arg(now)::timestamptz
        ELSE login_throttles.window_started_at
    END,
    blocked_until = CASE
        WHEN login_throttles.window_started_at <=
             sqlc.arg(now)::timestamptz - make_interval(secs => sqlc.arg(window_seconds)::integer)
            THEN NULL
        ELSE login_throttles.blocked_until
    END,
    expires_at = sqlc.arg(now)::timestamptz +
        make_interval(secs => sqlc.arg(retention_seconds)::integer)
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
