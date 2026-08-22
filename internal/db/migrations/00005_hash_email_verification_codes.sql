-- +goose Up
-- Existing links expire after 15 minutes and cannot be safely transformed
-- without retaining plaintext. Invalidate them so users request a fresh link.
UPDATE verify_emails
SET secret_code = repeat('0', 64);

-- +goose Down
-- SHA-256 digests cannot be reversed. The schema remains compatible, and
-- existing verification links must be reissued after rollback.
