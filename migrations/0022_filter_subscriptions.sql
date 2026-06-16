-- Filter subscriptions + notification fan-out. A user subscribes one of their
-- saved searches to a delivery channel and is pushed matching jobs. The matching
-- worker (cmd/notify) re-runs each DISTINCT saved-search query against Meilisearch
-- once per pass (cost O(distinct queries), not O(jobs x subscribers)), records
-- matches here, and delivers one digest per subscription per pass.
--
-- Applied by Postgres on first volume init (same as 0001) and read by sqlc as
-- schema source. Existing volumes/prod need a manual apply (the
-- versioned-migration-runner seam from AGENT.md remains open).

-- A subscription = a saved search (the filter of record) + where to deliver it +
-- since when. The saved search owns the query string, so a subscription stores no
-- bespoke filter — the worker reads ss.query and translates it the same way the
-- search API does.
CREATE TABLE IF NOT EXISTS subscriptions (
    id              BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    saved_search_id BIGINT      NOT NULL REFERENCES saved_searches (id) ON DELETE CASCADE,
    -- Delivery channel. 'telegram' is the only implementation today; 'webhook'/
    -- 'email' are the seam (a new Notifier impl, no schema change).
    channel         TEXT        NOT NULL DEFAULT 'telegram',
    -- Channel address. NULL for telegram (the recipient chat_id is resolved from
    -- telegram_links by user_id); a URL for webhook, an address for email.
    destination     TEXT,
    -- Soft on/off so a user can pause notifications without losing the subscription.
    active          BOOLEAN     NOT NULL DEFAULT true,
    -- History cutoff: only jobs that become matchable at/after this instant are
    -- delivered, so subscribing to a broad filter does not replay the backlog.
    start_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- At most one subscription per (saved search, channel): re-subscribing the same
    -- saved search to telegram is a no-op, not a duplicate.
    UNIQUE (saved_search_id, channel)
);

-- List-by-owner for the "My subscriptions" view.
CREATE INDEX IF NOT EXISTS subscriptions_user_idx
    ON subscriptions (user_id);

-- The match ledger AND the delivery queue, one row per (subscription, job). The
-- composite PK is the dedup key — the invariant "a job is delivered to a
-- subscription at most once" — so the worker can re-scan recent jobs freely and
-- INSERT ... ON CONFLICT DO NOTHING silently drops repeats. A row is pending
-- delivery while notified_at IS NULL; the delivery loop leases a subscription's
-- pending rows (stamps claimed_at) in a short transaction, sends the digest OUT
-- of that transaction so no network call is held inside a row lock, then stamps
-- notified_at. claimed_at is the lease (mirrors enrichment_outbox): an expired
-- lease is reclaimable, so a crashed send is retried with no separate reaper.
-- attempts/failed_at/last_error are the same retry/dead-letter bookkeeping.
CREATE TABLE IF NOT EXISTS subscription_matches (
    subscription_id BIGINT      NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    job_id          BIGINT      NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    matched_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    notified_at     TIMESTAMPTZ,            -- NULL = pending delivery
    claimed_at      TIMESTAMPTZ,            -- lease stamp; NULL = unleased
    attempts        INT         NOT NULL DEFAULT 0,
    failed_at       TIMESTAMPTZ,            -- non-NULL = dead-lettered, never retried
    last_error      TEXT        NOT NULL DEFAULT '',
    PRIMARY KEY (subscription_id, job_id)
);

-- The delivery claim scans only pending, live rows; the partial index keeps that
-- working set small regardless of how large the notified history grows.
CREATE INDEX IF NOT EXISTS subscription_matches_pending_idx
    ON subscription_matches (subscription_id)
    WHERE notified_at IS NULL AND failed_at IS NULL;

-- One linked Telegram chat per user. A bot cannot message a user who has not
-- started it, so chat_id is captured from the inbound /start (see telegram-notify
-- spec); telegram deliveries resolve the recipient from here by user_id. Kept out
-- of the users table so notification concerns do not touch the users SELECT *.
CREATE TABLE IF NOT EXISTS telegram_links (
    user_id   BIGINT      PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    chat_id   BIGINT      NOT NULL,
    linked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
