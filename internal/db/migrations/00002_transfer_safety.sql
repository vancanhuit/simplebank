-- +goose Up
-- Idempotency key makes POST /transfers safe to retry: a client that resends
-- the same request (e.g. after a network timeout) reuses the same key, and the
-- UNIQUE constraint collapses the retry onto the original transfer instead of
-- moving money twice.
ALTER TABLE transfers ADD COLUMN idempotency_key uuid;
-- Backfill any pre-existing rows with a synthetic key so the column can be made
-- NOT NULL. uuidv7() is available on PostgreSQL 18.
UPDATE transfers SET idempotency_key = uuidv7() WHERE idempotency_key IS NULL;
ALTER TABLE transfers ALTER COLUMN idempotency_key SET NOT NULL;
ALTER TABLE transfers ADD CONSTRAINT transfers_idempotency_key_key UNIQUE (idempotency_key);

-- +goose Down
ALTER TABLE transfers DROP CONSTRAINT transfers_idempotency_key_key;
ALTER TABLE transfers DROP COLUMN idempotency_key;
