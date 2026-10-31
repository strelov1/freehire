-- Interview buddies' reactions: one member's single reaction to one feed event (an
-- application, or a move into interview or offer). One row per (event, member) — choosing
-- another reaction replaces it, choosing the same one again removes it — so a member
-- counts once per event, as on every social feed.
--
-- The kind is a closed vocabulary chosen for a job search rather than a generic "like":
-- congrats (an offer), luck (an interview), rooting, same (same boat), insight. Who may
-- react to what is not a column: the service admits a reaction only to an event the
-- reacting member's own feed shows (internal/engage/buddy), so every exclusion the feed
-- keeps (blocks, hidden applications, private postings, members who left) holds here too.
--
-- Both references cascade: a deleted application takes its events and their reactions
-- with it, and a deleted account takes the reactions it gave. The primary key leads with
-- event_id, which serves the per-event counts; user_id gets its own index because every
-- foreign key to users must be indexed on the referencing side.
CREATE TABLE IF NOT EXISTS buddy_reactions (
    event_id   bigint      NOT NULL REFERENCES application_events (id) ON DELETE CASCADE,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       text        NOT NULL CHECK (kind IN ('congrats', 'luck', 'rooting', 'same', 'insight')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, user_id)
);

CREATE INDEX IF NOT EXISTS buddy_reactions_user_idx ON buddy_reactions (user_id);
