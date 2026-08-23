-- +goose Up
CREATE TABLE notifications (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    owner       text NOT NULL REFERENCES users(username),
    account_id  uuid NOT NULL REFERENCES accounts(id),
    transfer_id uuid NOT NULL REFERENCES transfers(id),
    direction   text NOT NULL CHECK (direction IN ('sent', 'received')),
    amount      bigint NOT NULL CHECK (amount > 0 AND amount <= 9007199254740991),
    currency    text NOT NULL,
    balance     bigint NOT NULL CHECK (balance BETWEEN 0 AND 9007199254740991),
    read_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (transfer_id, account_id)
);

CREATE INDEX idx_notifications_owner_history
ON notifications (owner, created_at DESC, id DESC);

CREATE INDEX idx_notifications_owner_unread
ON notifications (owner)
WHERE read_at IS NULL;

-- +goose Down
DROP TABLE notifications;
