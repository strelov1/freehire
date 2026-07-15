-- Inbox soft-delete: a deleted message is hidden from the listing but retained
-- (recoverable via Undo / restore). Gmail's UpsertEmail is ON CONFLICT DO NOTHING,
-- so a re-synced message never clears this flag — soft-delete is durable.
--
-- initdb applies this once on a fresh volume; on an existing prod volume apply it
-- by hand before deploying the new binary (no versioned runner yet).

ALTER TABLE emails ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- The per-user newest-first listing only ever reads live rows; a partial index
-- keeps that scan off soft-deleted tombstones.
CREATE INDEX IF NOT EXISTS emails_user_live_idx
    ON emails (user_id, received_at DESC) WHERE deleted_at IS NULL;
