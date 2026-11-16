ALTER TABLE gmail_connections ADD COLUMN gmail_retry_after timestamptz NOT NULL DEFAULT '-infinity';
