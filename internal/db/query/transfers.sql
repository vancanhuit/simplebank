-- name: CreateTransfer :one
INSERT INTO transfers (from_account_id, to_account_id, amount)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListTransfersByAccount :many
SELECT * FROM transfers
WHERE from_account_id = sqlc.arg(account_id) OR to_account_id = sqlc.arg(account_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
