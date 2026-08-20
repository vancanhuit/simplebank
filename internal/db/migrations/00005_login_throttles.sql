-- +goose Up
CREATE TABLE login_throttles (
    scope             text NOT NULL CHECK (scope IN ('account', 'client')),
    key_hash          text NOT NULL,
    failure_count     integer NOT NULL CHECK (failure_count > 0),
    window_started_at timestamptz NOT NULL,
    blocked_until     timestamptz,
    expires_at        timestamptz NOT NULL,
    PRIMARY KEY (scope, key_hash)
);

CREATE INDEX idx_login_throttles_expires_at
    ON login_throttles (expires_at);

-- +goose Down
DROP TABLE login_throttles;
