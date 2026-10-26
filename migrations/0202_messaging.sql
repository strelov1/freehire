-- The messaging core (openspec/changes/messaging-core): conversations between accounts,
-- shared by every feature that lets people write to each other. A conversation names the
-- feature it belongs to in `kind`; the feature decides who may start one and how the other
-- side is shown, the core holds everything else.

-- pair_key is the kind's own uniqueness rule: a two-person kind sets it from the ordered
-- pair, so the pair keeps one conversation of that kind; a kind without such a rule leaves
-- it NULL, which UNIQUE never compares equal. starter_id is who sent the first message,
-- the only one who may NOT accept it.
CREATE TABLE IF NOT EXISTS conversations (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind        text        NOT NULL CHECK (kind IN ('buddy')),
    pair_key    text,
    starter_id  bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status      text        NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'open', 'declined', 'closed')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    accepted_at timestamptz,
    closed_at   timestamptz,
    UNIQUE (kind, pair_key)
);

-- A kind's rolling start count (the buddy invitation cap), and the starter's cascade.
CREATE INDEX IF NOT EXISTS conversations_starter_kind_created_idx
    ON conversations (starter_id, kind, created_at);

-- One row per account in a conversation, carrying the PERSONA the account speaks as there,
-- its own read cursor (the newest message id it has read) and when it was last told about
-- unread messages. One account has several personas — its real self, an anonymous buddy
-- handle, later a mentor or an employer — and the other side is drawn from this column,
-- never from the account: what one persona shows must not lead to another of the same
-- person.
CREATE TABLE IF NOT EXISTS conversation_participants (
    conversation_id      bigint      NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    user_id              bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    persona              text        NOT NULL CHECK (persona IN ('buddy')),
    last_read_message_id bigint,
    notified_at          timestamptz,
    PRIMARY KEY (conversation_id, user_id)
);

-- A member's conversation list and the account-deletion cascade.
CREATE INDEX IF NOT EXISTS conversation_participants_user_idx ON conversation_participants (user_id);

-- Every conversation today is between two people, and deleting either account deletes the
-- conversation with all its messages: the one left behind must not keep the other's words.
-- The participant row cascades away with the account; this takes the conversation with it.
-- A kind with more than two participants will have to revisit this (drop the participant,
-- keep the thread) — see the messaging-core design, Risks.
CREATE OR REPLACE FUNCTION public.conversation_participant_deleted() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM conversations WHERE id = OLD.conversation_id;
    RETURN NULL;
END;
$$;

CREATE TRIGGER conversation_participant_deleted
    AFTER DELETE ON conversation_participants
    FOR EACH ROW EXECUTE FUNCTION public.conversation_participant_deleted();

CREATE TABLE IF NOT EXISTS messages (
    id              bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id bigint      NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    sender_id       bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body            text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 4000),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS messages_conversation_idx ON messages (conversation_id, id);
CREATE INDEX IF NOT EXISTS messages_sender_idx ON messages (sender_id);

-- A block is between two accounts, never tied to a kind: someone blocked in one feature
-- must not reach the blocker through another. Read in either direction.
CREATE TABLE IF NOT EXISTS user_blocks (
    blocker_id bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    blocked_id bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id)
);

CREATE INDEX IF NOT EXISTS user_blocks_blocked_idx ON user_blocks (blocked_id);

-- A report of a conversation by one of its participants. The reason is a closed list.
CREATE TABLE IF NOT EXISTS conversation_reports (
    id              bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id bigint      NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    reporter_id     bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    reason          text        NOT NULL CHECK (reason IN ('spam', 'harassment', 'scam', 'other')),
    note            text        CHECK (note IS NULL OR char_length(note) <= 1000),
    created_at      timestamptz NOT NULL DEFAULT now(),
    resolved_at     timestamptz,
    resolved_by     bigint      REFERENCES users (id) ON DELETE SET NULL,
    UNIQUE (conversation_id, reporter_id)
);

CREATE INDEX IF NOT EXISTS conversation_reports_reporter_idx ON conversation_reports (reporter_id);
CREATE INDEX IF NOT EXISTS conversation_reports_resolved_by_idx
    ON conversation_reports (resolved_by) WHERE resolved_by IS NOT NULL;
