-- +goose Up
ALTER TABLE transfers DROP CONSTRAINT transfers_idempotency_key_key;
ALTER TABLE transfers
    ADD CONSTRAINT transfers_from_account_id_idempotency_key_key
    UNIQUE (from_account_id, idempotency_key);

-- +goose Down
-- Reverting to the original global idempotency key constraint is only safe when
-- every existing key is globally unique. The scoped constraint introduced in
-- this migration permits the same key for different source accounts, so check
-- first and leave the current constraint intact if rollback would fail or lose
-- the operator's ability to inspect the conflicting rows.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM transfers
        GROUP BY idempotency_key
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot rollback 00003_scope_transfer_idempotency: duplicate transfer idempotency keys exist across source accounts; resolve duplicates before restoring global uniqueness';
    END IF;
END $$;

ALTER TABLE transfers DROP CONSTRAINT transfers_from_account_id_idempotency_key_key;
ALTER TABLE transfers
    ADD CONSTRAINT transfers_idempotency_key_key UNIQUE (idempotency_key);
