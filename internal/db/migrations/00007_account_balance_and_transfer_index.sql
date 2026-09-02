-- +goose Up
-- +goose NO TRANSACTION

SET lock_timeout = '5s';

-- +goose StatementBegin
DO
$$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'accounts'::regclass
          AND conname = 'accounts_balance_nonnegative'
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_balance_nonnegative CHECK (balance >= 0) NOT VALID;
    END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE accounts VALIDATE CONSTRAINT accounts_balance_nonnegative;

-- +goose StatementBegin
DO
$$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_index
        WHERE indexrelid = to_regclass('idx_transfers_from_account_created_at')
          AND NOT indisvalid
    ) THEN
        DROP INDEX idx_transfers_from_account_created_at;
    END IF;
END
$$;
-- +goose StatementEnd

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_from_account_created_at
    ON transfers (from_account_id, created_at);

RESET lock_timeout;

-- +goose Down
SET lock_timeout = '5s';
DROP INDEX CONCURRENTLY IF EXISTS idx_transfers_from_account_created_at;
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_balance_nonnegative;
RESET lock_timeout;
