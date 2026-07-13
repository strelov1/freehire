-- Hosted mailbox + unified mail store. Adds a per-user address on our receiving
-- domain (mailboxes) and generalizes the Gmail-specific emails table into a
-- source-agnostic message store, so Gmail-synced mail and mail received at a
-- hosted address share one inbox.
--
-- 0014 is already live on prod with the user's synced Gmail rows, so this
-- refactors emails IN PLACE: additive columns + a column rename + a widened
-- uniqueness. Existing rows are preserved and backfilled to source='gmail' by
-- the column default. initdb applies this once on a fresh volume; on an existing
-- prod volume apply it by hand BEFORE deploying the new binary (no versioned
-- runner — an unapplied rename would 42703 every inbox read).

-- Per-user address on the receiving domain (<handle>@MAIL_DOMAIN).
CREATE TABLE IF NOT EXISTS mailboxes (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- One mailbox per user; the FK cascade releases it on user delete.
    user_id    BIGINT      NOT NULL UNIQUE REFERENCES users (id) ON DELETE CASCADE,
    address    TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Generalize emails into a source-agnostic store. source discriminates the
-- ingest path; external_id is the per-source dedup key (Gmail message id for
-- 'gmail', RFC Message-ID for 'hosted'); s3_key points at the raw MIME for
-- hosted mail (NULL for Gmail); read_at stamps when the body was first opened.
ALTER TABLE emails ADD COLUMN source TEXT NOT NULL DEFAULT 'gmail';
ALTER TABLE emails RENAME COLUMN gmail_msg_id TO external_id;
ALTER TABLE emails ADD COLUMN s3_key TEXT;
ALTER TABLE emails ADD COLUMN read_at TIMESTAMPTZ;

-- Widen dedup from (user, gmail id) to (user, source, external id) so the two
-- sources can't collide and each stays idempotent on re-ingest.
ALTER TABLE emails DROP CONSTRAINT IF EXISTS emails_user_id_gmail_msg_id_key;
ALTER TABLE emails ADD CONSTRAINT emails_user_source_external_key
    UNIQUE (user_id, source, external_id);
