-- +goose Up
-- +goose StatementBegin
DO
$$
BEGIN
    IF EXISTS (SELECT 1 FROM accounts WHERE balance > 9007199254740991) THEN
        RAISE EXCEPTION 'cannot apply JavaScript-safe balance constraint: unsafe balances exist';
    END IF;

    IF EXISTS (SELECT 1 FROM transfers WHERE amount > 9007199254740991) THEN
        RAISE EXCEPTION 'cannot apply JavaScript-safe transfer amount constraint: unsafe transfer amounts exist';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM entries
        WHERE amount < -9007199254740991 OR amount > 9007199254740991
    ) THEN
        RAISE EXCEPTION 'cannot apply JavaScript-safe entry amount constraint: unsafe entry amounts exist';
    END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE accounts
    ADD CONSTRAINT accounts_balance_javascript_safe
    CHECK (balance <= 9007199254740991);

ALTER TABLE transfers
    ADD CONSTRAINT transfers_amount_javascript_safe
    CHECK (amount <= 9007199254740991);

ALTER TABLE entries
    ADD CONSTRAINT entries_amount_javascript_safe
    CHECK (amount BETWEEN -9007199254740991 AND 9007199254740991);

-- +goose Down
ALTER TABLE transfers DROP CONSTRAINT IF EXISTS transfers_amount_javascript_safe;
ALTER TABLE entries DROP CONSTRAINT IF EXISTS entries_amount_javascript_safe;
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_balance_javascript_safe;
