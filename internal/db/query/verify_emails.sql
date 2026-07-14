-- name: CreateVerifyEmail :one
INSERT INTO verify_emails (username, email, secret_code)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateVerifyEmail :one
UPDATE verify_emails
SET is_used = true
WHERE id = sqlc.arg(id)
  AND secret_code = sqlc.arg(secret_code)
  AND is_used = false
  AND expired_at > now()
RETURNING *;
