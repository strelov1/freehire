-- Interview buddies (openspec/changes/interview-buddies): an opt-in anonymous peer
-- network. A member is known to others only by a generated handle. Their conversations,
-- blocks and reports live in the messaging core (0202_messaging.sql) under kind 'buddy';
-- every foreign key to users cascades, so account deletion removes the membership too.

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

-- Applications a member keeps out of the feed, keyed on the application (the key every
-- ledger event carries in application_events.application_id and which survives cmd/prune,
-- unlike job_id). There is deliberately NO foreign key on application_id: a reference
-- would take a lock on applications inside the migration transaction, and a mark left
-- behind by a deleted application is harmless because that application's events cascade
-- away with it.
CREATE TABLE IF NOT EXISTS buddy_hidden_applications (
    user_id        bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    application_id bigint      NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, application_id)
);
