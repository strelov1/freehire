-- Interview buddies (openspec/changes/interview-buddies): an opt-in anonymous peer
-- network. A member is known to others only by a generated handle; conversations are
-- one per pair; every foreign key to users cascades, so account deletion removes the
-- membership, the conversations and their messages with it.

-- Membership. The row outlives leaving (left_at) so that rejoining restores the handle.
-- category/countries/regions are the ranking keys, recomputed from the CV and profile;
-- they are stored because similarity is scored in SQL.
CREATE TABLE IF NOT EXISTS buddy_members (
    user_id   bigint      PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    handle    text        NOT NULL UNIQUE,
    category  text,
    countries text[]      NOT NULL DEFAULT '{}',
    regions   text[]      NOT NULL DEFAULT '{}',
    joined_at timestamptz NOT NULL DEFAULT now(),
    left_at   timestamptz
);

-- Applications a member keeps out of the feed, keyed by job_id like the ledger.
-- job_id has its own index: cmd/prune hard-deletes jobs and the cascade would
-- otherwise scan this table once per deleted job.
CREATE TABLE IF NOT EXISTS buddy_hidden_jobs (
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    job_id     bigint      NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, job_id)
);

CREATE INDEX IF NOT EXISTS buddy_hidden_jobs_job_idx ON buddy_hidden_jobs (job_id);

-- One conversation per pair: user_low < user_high plus the unique constraint makes the
-- pair order-independent. The read cursors (last message id read) and the notified
-- stamps are one per side; with two participants a side table would only add a join.
CREATE TABLE IF NOT EXISTS buddy_conversations (
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_low      bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    user_high     bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    initiator_id  bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status        text        NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'open', 'declined', 'closed')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    accepted_at   timestamptz,
    closed_at     timestamptz,
    last_read_low  bigint,
    last_read_high bigint,
    notified_low_at  timestamptz,
    notified_high_at timestamptz,
    CHECK (user_low < user_high),
    CHECK (initiator_id IN (user_low, user_high)),
    UNIQUE (user_low, user_high)
);

-- The other participant's side of the account-deletion cascade (user_low leads the
-- unique constraint above).
CREATE INDEX IF NOT EXISTS buddy_conversations_high_idx ON buddy_conversations (user_high);

-- The rolling 24-hour invitation count, and the initiator's cascade.
CREATE INDEX IF NOT EXISTS buddy_conversations_initiator_created_idx
    ON buddy_conversations (initiator_id, created_at);

CREATE TABLE IF NOT EXISTS buddy_messages (
    id              bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id bigint      NOT NULL REFERENCES buddy_conversations (id) ON DELETE CASCADE,
    sender_id       bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body            text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 4000),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS buddy_messages_conversation_idx ON buddy_messages (conversation_id, id);
CREATE INDEX IF NOT EXISTS buddy_messages_sender_idx ON buddy_messages (sender_id);

-- A block hides the pair from each other everywhere, in either direction.
CREATE TABLE IF NOT EXISTS buddy_blocks (
    blocker_id bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    blocked_id bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id)
);

CREATE INDEX IF NOT EXISTS buddy_blocks_blocked_idx ON buddy_blocks (blocked_id);

-- A report of a conversation, from one of its participants. The reason is a closed list.
CREATE TABLE IF NOT EXISTS buddy_reports (
    id              bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id bigint      NOT NULL REFERENCES buddy_conversations (id) ON DELETE CASCADE,
    reporter_id     bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    reason          text        NOT NULL CHECK (reason IN ('spam', 'harassment', 'scam', 'other')),
    note            text        CHECK (note IS NULL OR char_length(note) <= 1000),
    created_at      timestamptz NOT NULL DEFAULT now(),
    resolved_at     timestamptz,
    resolved_by     bigint      REFERENCES users (id) ON DELETE SET NULL,
    UNIQUE (conversation_id, reporter_id)
);

CREATE INDEX IF NOT EXISTS buddy_reports_reporter_idx ON buddy_reports (reporter_id);
CREATE INDEX IF NOT EXISTS buddy_reports_resolved_by_idx ON buddy_reports (resolved_by) WHERE resolved_by IS NOT NULL;
