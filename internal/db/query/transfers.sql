-- name: CreateTransfer :one
INSERT INTO transfers (from_account_id, to_account_id, amount, idempotency_key)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetTransferByIdempotencyKey :one
SELECT * FROM transfers WHERE idempotency_key = $1 LIMIT 1;

-- name: SumOutgoingTransfersSince :one
SELECT COALESCE(SUM(amount), 0)::bigint AS total
FROM transfers
WHERE from_account_id = sqlc.arg(from_account_id) AND created_at >= sqlc.arg(since);

-- name: ListTransfersByAccount :many
SELECT * FROM transfers
WHERE from_account_id = sqlc.arg(account_id) OR to_account_id = sqlc.arg(account_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
