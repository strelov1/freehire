-- Gmail ATS inbox: per-user Gmail connection (encrypted refresh token + sync
-- cursor) and the stored ATS mail. Both live in hire's main DB, so they carry
-- real foreign keys to users (disconnect / user delete cascades cleanly).
--
-- initdb applies this once on a fresh volume; on an existing prod volume apply it
-- by hand before deploying the new binary (no versioned runner yet).

CREATE TABLE IF NOT EXISTS gmail_connections (
    -- One connection per user; the FK cascade purges it on user delete.
    user_id           BIGINT      PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    email             TEXT        NOT NULL,
    -- AES-256-GCM ciphertext (base64) of the OAuth refresh token; never plaintext.
    refresh_token_enc TEXT        NOT NULL,
    -- connected | needs_reconsent (a revoked/expired grant flips this).
    status            TEXT        NOT NULL DEFAULT 'connected',
    -- Incremental sync watermark: the Unix time of the newest synced message.
    sync_cursor       BIGINT      NOT NULL DEFAULT 0,
    connected_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_synced_at    TIMESTAMPTZ
);

-- One row per stored ATS email. gmail_msg_id is unique per user so a re-sync is
-- idempotent; subject_norm is the inbox grouping key (see gmailsync.NormalizeSubject).
CREATE TABLE IF NOT EXISTS emails (
    id           BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    gmail_msg_id TEXT        NOT NULL,
    thread_id    TEXT        NOT NULL DEFAULT '',
    from_addr    TEXT        NOT NULL DEFAULT '',
    from_name    TEXT        NOT NULL DEFAULT '',
    subject      TEXT        NOT NULL DEFAULT '',
    subject_norm TEXT        NOT NULL DEFAULT '',
    body_text    TEXT        NOT NULL DEFAULT '',
    body_html    TEXT        NOT NULL DEFAULT '',
    received_at  TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, gmail_msg_id)
);

-- Inbox grouping (by subject_norm) and the per-user newest-first listing.
CREATE INDEX IF NOT EXISTS emails_user_subject_idx  ON emails (user_id, subject_norm);
CREATE INDEX IF NOT EXISTS emails_user_received_idx ON emails (user_id, received_at DESC);
