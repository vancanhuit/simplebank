-- name: CreateSession :one
INSERT INTO sessions (id, username, refresh_token, user_agent, client_ip, is_blocked, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1 LIMIT 1;

-- name: GetSessionForUpdate :one
SELECT * FROM sessions WHERE id = $1 LIMIT 1 FOR UPDATE;

-- name: RotateSession :one
UPDATE sessions
SET refresh_token = sqlc.arg(new_refresh_token),
    expires_at = sqlc.arg(new_expires_at)
WHERE id = sqlc.arg(old_id)
RETURNING *;

-- name: BlockSession :one
UPDATE sessions
SET is_blocked = true
WHERE id = sqlc.arg(id)
RETURNING *;
