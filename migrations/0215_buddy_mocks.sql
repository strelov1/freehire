-- Buddy mock interviews (openspec/changes/buddy-mock-interviews): one interview-buddy member
-- books a 60-minute peer mock from another's published availability. Every reference to
-- users cascades, and every one is indexed on this side (TestEveryUserForeignKeyIsIndexed).

-- btree_gist backs the participants EXCLUDE below (a scalar = beside a range &&). 0145
-- already created it; repeated so this file stands on its own.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- A member's mock-host settings. timezone is an IANA name, validated where it is read, for
-- the reason 0145 gives for mentors.timezone. The link is the host's own call room; only
-- the two participants of a confirmed mock ever read it, and it is read live rather than
-- snapshotted, so a host who fixes a typo reaches guests who already booked.
CREATE TABLE IF NOT EXISTS buddy_mock_settings (
    user_id     bigint      PRIMARY KEY REFERENCES buddy_members (user_id) ON DELETE CASCADE,
    timezone    text        NOT NULL,
    meeting_url text        NOT NULL,
    topics      text[]      NOT NULL,
    paused      boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT buddy_mock_settings_url_check
        CHECK (meeting_url LIKE 'https://%' AND char_length(meeting_url) <= 500),
    CONSTRAINT buddy_mock_settings_topics_check
        CHECK (cardinality(topics) > 0
               AND topics <@ ARRAY['coding', 'system_design', 'behavioral']::text[])
);

-- Availability in the two row shapes mentor_availability has (0145): a weekly row names a
-- weekday, a dated row names one date and replaces that date's weekly rows; an empty dated
-- row closes its date. Times carry no zone: they resolve through the host's settings zone.
CREATE TABLE IF NOT EXISTS buddy_mock_availability (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES buddy_members (user_id) ON DELETE CASCADE,
    -- A day of the week, 0 through 6.
    -- squawk-ignore prefer-bigint-over-smallint
    weekday    smallint,
    on_date    date,
    start_time time        NOT NULL,
    end_time   time        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT buddy_mock_availability_shape_check
        CHECK ((weekday IS NULL) <> (on_date IS NULL)),
    CONSTRAINT buddy_mock_availability_weekday_check
        CHECK (weekday IS NULL OR weekday BETWEEN 0 AND 6),
    CONSTRAINT buddy_mock_availability_span_check
        CHECK (end_time >= start_time AND (on_date IS NOT NULL OR end_time > start_time))
);

CREATE INDEX IF NOT EXISTS buddy_mock_availability_user_idx ON buddy_mock_availability (user_id);

-- One booked mock. The id is a random uuid because two accounts read it. There is no
-- 'completed' status: completion is "confirmed and ended". guest_timezone is what the
-- guest's notices are rendered in; the host's is read from their settings. cancel_reason
-- picks the wording of the cancellation notice, and cancel_announced_at is that notice's
-- claim: a block cancels inside a trigger, which cannot hand rows back to the caller.
CREATE TABLE IF NOT EXISTS buddy_mocks (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    host_user_id        bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    guest_user_id       bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    starts_at           timestamptz NOT NULL,
    ends_at             timestamptz NOT NULL,
    topic               text        NOT NULL,
    guest_timezone      text        NOT NULL,
    status              text        NOT NULL DEFAULT 'confirmed',
    cancelled_by        bigint      REFERENCES users (id) ON DELETE SET NULL,
    cancelled_at        timestamptz,
    cancel_reason       text,
    cancel_announced_at timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT buddy_mocks_pair_check   CHECK (host_user_id <> guest_user_id),
    CONSTRAINT buddy_mocks_span_check   CHECK (ends_at > starts_at),
    CONSTRAINT buddy_mocks_topic_check  CHECK (topic IN ('coding', 'system_design', 'behavioral')),
    CONSTRAINT buddy_mocks_status_check CHECK (status IN ('confirmed', 'cancelled')),
    CONSTRAINT buddy_mocks_cancelled_check
        CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL)
               AND (status = 'cancelled') = (cancel_reason IS NOT NULL)),
    CONSTRAINT buddy_mocks_cancel_reason_check
        CHECK (cancel_reason IN ('participant', 'left', 'blocked'))
);

-- Each member's own list, either role, and account deletion.
CREATE INDEX IF NOT EXISTS buddy_mocks_host_starts_at_idx ON buddy_mocks (host_user_id, starts_at);
CREATE INDEX IF NOT EXISTS buddy_mocks_guest_starts_at_idx ON buddy_mocks (guest_user_id, starts_at);
-- Account deletion and the cancellation-notice claim. Partial: a live mock has nobody who
-- cancelled it.
CREATE INDEX IF NOT EXISTS buddy_mocks_cancelled_by_idx
    ON buddy_mocks (cancelled_by) WHERE cancelled_by IS NOT NULL;
-- The reminder worker's scan.
CREATE INDEX IF NOT EXISTS buddy_mocks_upcoming_idx
    ON buddy_mocks (starts_at) WHERE status = 'confirmed';

-- One row per person per mock, so ONE constraint can refuse "host at 19:00 and guest at
-- 19:00": a mentor booking only constrains the mentor, while here either person may hold
-- either role. Half-open '[)' bounds match slots.Interval.Overlaps, so back-to-back mocks
-- are legal. The WHERE is load-bearing: a cancelled mock sets active = false in the same
-- statement as its status, and frees both people's time.
CREATE TABLE IF NOT EXISTS buddy_mock_participants (
    mock_id   uuid        NOT NULL REFERENCES buddy_mocks (id) ON DELETE CASCADE,
    user_id   bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    starts_at timestamptz NOT NULL,
    ends_at   timestamptz NOT NULL,
    active    boolean     NOT NULL DEFAULT true,

    PRIMARY KEY (mock_id, user_id),
    CONSTRAINT buddy_mock_participants_span_check CHECK (ends_at > starts_at),
    CONSTRAINT buddy_mock_participants_no_overlap
        EXCLUDE USING gist (
            user_id WITH =,
            tstzrange(starts_at, ends_at, '[)') WITH &&
        ) WHERE (active)
);

-- A member's busy time for the slot engine, and account deletion (the EXCLUDE's index is
-- partial on active and cannot serve the cascade).
CREATE INDEX IF NOT EXISTS buddy_mock_participants_user_idx
    ON buddy_mock_participants (user_id, starts_at);

-- One review per participant per mock; the key is what makes a second one impossible.
-- Revealed when the counterpart has reviewed too or seven days after the end — computed in
-- the read queries, never stored.
CREATE TABLE IF NOT EXISTS buddy_mock_reviews (
    mock_id         uuid        NOT NULL REFERENCES buddy_mocks (id) ON DELETE CASCADE,
    author_user_id  bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    subject_user_id bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- One to five stars.
    -- squawk-ignore prefer-bigint-over-smallint
    rating          smallint    NOT NULL,
    comment         text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (mock_id, author_user_id),
    CONSTRAINT buddy_mock_reviews_rating_check  CHECK (rating BETWEEN 1 AND 5),
    CONSTRAINT buddy_mock_reviews_comment_check CHECK (char_length(comment) <= 1000),
    CONSTRAINT buddy_mock_reviews_pair_check    CHECK (author_user_id <> subject_user_id)
);

CREATE INDEX IF NOT EXISTS buddy_mock_reviews_author_idx ON buddy_mock_reviews (author_user_id);
-- The member page's aggregate and recent reviews.
CREATE INDEX IF NOT EXISTS buddy_mock_reviews_subject_idx
    ON buddy_mock_reviews (subject_user_id, created_at DESC);

-- What cmd/mentorship-remind already sent for a mock; the composite key is the claim, as
-- in mentor_booking_reminders.
CREATE TABLE IF NOT EXISTS buddy_mock_reminders (
    mock_id        uuid        NOT NULL REFERENCES buddy_mocks (id) ON DELETE CASCADE,
    -- Minutes before the mock: 1440 and 60 today. A label, not a counter.
    -- squawk-ignore prefer-bigint-over-int
    offset_minutes integer     NOT NULL,
    sent_at        timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (mock_id, offset_minutes),
    CONSTRAINT buddy_mock_reminders_offset_check CHECK (offset_minutes > 0)
);

-- A block cancels the pair's future mocks inside the block's own statement. The block is
-- written by the messaging core (BlockFromConversation), which never imports a feature, so
-- the feature reacts here; every path that ever inserts a block is covered by construction.
-- cancelled_by is the blocker, which is how the caller claims the notice afterwards.
--
-- The block's half of the buddy_members lock protocol (the booking's is
-- LockBuddyMembersForMock, the leave's LockBuddyMemberForLeave): lock both people's rows
-- FOR UPDATE in user_id order, the order a booking takes its share locks, and only then
-- cancel, in a separate statement. A booking holding its share locks makes this wait until
-- the mock is committed; under READ COMMITTED each statement of a plpgsql function takes a
-- fresh snapshot, so the cancel after the wait sees that mock. A booking arriving after
-- this lock waits for the block to commit and then sees the block. A non-member has no
-- row, which locks nothing and is fine: there is no mock to race.
CREATE OR REPLACE FUNCTION public.buddy_mocks_cancel_on_block() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM 1 FROM buddy_members
    WHERE user_id IN (NEW.blocker_id, NEW.blocked_id)
    ORDER BY user_id
    FOR UPDATE;

    WITH cancelled AS (
        UPDATE buddy_mocks m
        SET status = 'cancelled', cancelled_at = now(), cancelled_by = NEW.blocker_id,
            cancel_reason = 'blocked'
        WHERE m.status = 'confirmed'
          AND m.starts_at > now()
          AND ((m.host_user_id = NEW.blocker_id AND m.guest_user_id = NEW.blocked_id)
            OR (m.host_user_id = NEW.blocked_id AND m.guest_user_id = NEW.blocker_id))
        RETURNING m.id
    )
    UPDATE buddy_mock_participants p
    SET active = false
    WHERE p.mock_id IN (SELECT id FROM cancelled);
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER buddy_mocks_cancel_on_block
    AFTER INSERT ON user_blocks
    FOR EACH ROW EXECUTE FUNCTION public.buddy_mocks_cancel_on_block();
