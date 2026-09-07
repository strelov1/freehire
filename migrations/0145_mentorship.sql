-- The mentorship marketplace: a named, moderated insider at a company in the catalogue
-- publishes when they are free, and a seeker books half an hour of it. See the
-- mentorship-marketplace change for the reasoning; what follows is the part of it the
-- schema is responsible for.
--
-- This is the first thing in the repository that schedules anything. Three ideas below
-- are load-bearing and each is argued where it appears: availability stored WITHOUT a
-- zone, an override that closes a day by being empty, and a non-overlap guarantee that
-- lives in the database rather than in Go.
--
-- btree_gist is required by the booking non-overlap constraint, which puts a scalar and
-- a range in one GiST index. It is a stock contrib extension, available on prod at
-- version 1.8 and not previously installed; pg_trgm (0033) and vector (0092) established
-- that this database may install one.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- mentors — one profile per account, naming one company by slug, the same key
-- jobs.company_slug and referral_offers.company_slug carry.
--
-- WHY status AND paused ARE SEPARATE COLUMNS. They record two different people's
-- decisions. status is the moderator's: pending until a human looks, then approved or
-- rejected. paused is the mentor's own, and needs no moderator to set or clear. Folding
-- the two into one enum would mean a mentor resuming has to remember what the moderator
-- had decided, and a moderator acting on a paused profile would silently unpause it.
--
-- WHY THE SESSION PARAMETERS LIVE HERE and not in a mentor_session_types table: a mentor
-- offers ONE session. cal.com models event types as a table because selling configurable
-- event types is their product; here it would cost a join on every slot computation and
-- a "which type?" step in booking, to serve a distinction nobody has asked for. When a
-- second type is genuinely needed these five columns move to a new table and
-- mentor_bookings gains a session_type_id — the Go slot engine already takes the
-- parameters as an argument rather than reading them off a mentor, so it does not change.
--
-- WHY timezone IS text AND NOT VALIDATED HERE. It is an IANA name, and the set of valid
-- names is the zone database on the host, which Postgres cannot express as a CHECK and
-- which changes underneath us. It is validated where it is read (time.LoadLocation), and
-- a name that stops resolving must fail loudly for that mentor rather than silently
-- become UTC — see internal/engage/mentorship/slots.go.
CREATE TABLE public.mentors (
    id                   bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id              bigint      NOT NULL UNIQUE REFERENCES public.users(id) ON DELETE CASCADE,
    company_slug         text        NOT NULL REFERENCES public.companies(slug) ON DELETE CASCADE,
    slug                 text        NOT NULL UNIQUE,
    -- The name the public sees. On the PROFILE rather than read from the account,
    -- because `users` carries no name at all — only an email and a username, and a
    -- username is an address rather than a name. This is also the column that makes a
    -- mentor the opposite of a referrer: a referral offer is anonymous on purpose, a
    -- mentor is chosen and therefore has a face.
    display_name         text        NOT NULL,
    headline             text        NOT NULL,
    bio                  text        NOT NULL DEFAULT '',
    topics               text[]      NOT NULL DEFAULT '{}',
    languages            text[]      NOT NULL DEFAULT '{}',
    timezone             text        NOT NULL,
    -- The five session figures below are minutes and days a human typed into a form, each
    -- bounded by a CHECK further down and by the UI above it. The 32-bit limit is 4085
    -- years of minutes; a mentor reaching it has not overflowed a counter, they have
    -- entered nonsense, and that is the CHECK's job rather than the column width's. Same
    -- reading as auto_apply_queue.attempts (migration 0116).
    -- squawk-ignore prefer-bigint-over-int
    session_duration_min integer     NOT NULL,
    -- squawk-ignore prefer-bigint-over-int
    buffer_before_min    integer     NOT NULL DEFAULT 0,
    -- squawk-ignore prefer-bigint-over-int
    buffer_after_min     integer     NOT NULL DEFAULT 0,
    -- squawk-ignore prefer-bigint-over-int
    min_notice_min       integer     NOT NULL DEFAULT 120,
    -- squawk-ignore prefer-bigint-over-int
    horizon_days         integer     NOT NULL DEFAULT 30,
    meeting_url          text        NOT NULL,
    status               text        NOT NULL DEFAULT 'pending',
    paused               boolean     NOT NULL DEFAULT false,
    decided_by           bigint      REFERENCES public.users(id) ON DELETE SET NULL,
    decided_at           timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),

    -- 'withdrawn' is how a mentor LEAVES, and the row survives it. Deleting the row
    -- instead would take every booking and review with it through the ON DELETE CASCADEs
    -- below — and a session that happened is history both parties are entitled to, not an
    -- artefact of the mentor still being on the platform.
    CONSTRAINT mentors_status_check
        CHECK (status IN ('pending', 'approved', 'rejected', 'withdrawn')),
    -- The same bounds internal/engage/mentorship's SessionParams.Validate enforces. Both
    -- exist deliberately: the Go check gives a caller a reason, this one holds for any
    -- writer a later change adds.
    CONSTRAINT mentors_duration_check CHECK (session_duration_min > 0),
    CONSTRAINT mentors_buffers_check  CHECK (buffer_before_min >= 0 AND buffer_after_min >= 0),
    CONSTRAINT mentors_notice_check   CHECK (min_notice_min >= 0),
    CONSTRAINT mentors_horizon_check  CHECK (horizon_days > 0),
    CONSTRAINT mentors_slug_check     CHECK (slug <> ''),
    CONSTRAINT mentors_name_check     CHECK (display_name <> '')
);

COMMENT ON TABLE public.mentors IS
    'One moderated, publicly named mentor profile per account, bound to one company in '
    'the catalogue. The opposite number of referral_offers, which keeps its insider '
    'anonymous: a referral is a favour asked of a stranger, a mentor is chosen.';

COMMENT ON COLUMN public.mentors.timezone IS
    'IANA zone name. It alone gives mentor_availability''s zoneless times a meaning, so a '
    'row without a resolvable one has no schedule at all rather than a UTC one.';

COMMENT ON COLUMN public.mentors.paused IS
    'The mentor''s own switch, independent of the moderator''s status. A paused profile '
    'leaves the directory and offers no slots; its confirmed bookings stand.';

-- The public directory and the "does this company have a mentor?" check the vacancy page
-- runs. Partial, because the only rows either question can ever return are these.
CREATE INDEX mentors_company_published_idx
    ON public.mentors (company_slug)
    WHERE status = 'approved' AND NOT paused;

-- One profile per account, and a withdrawn one still holds the slot: coming back is not
-- in scope, and a second profile for an account that already has history would give one
-- person two identities in the same directory.


-- The moderation queue: pending profiles, oldest first, the same shape
-- referral_offers_pending_created_at_idx has.
CREATE INDEX mentors_pending_created_at_idx
    ON public.mentors (created_at)
    WHERE status = 'pending';

-- EVERY foreign key pointing at users must be indexed on THIS side. Postgres indexes the
-- referenced side only, so an unindexed reference turns each DELETE FROM users into a
-- sequential scan of this table — one per constraint. That is what made account deletion
-- outlive the proxy's read timeout in production once already, and
-- TestEveryUserForeignKeyIsIndexed is what caught these three.
--
-- Partial, because decided_by is NULL for every profile a moderator has not yet reached,
-- and the FK check looks for a match — which NULL never is. The index carries only the
-- rows that can answer.
CREATE INDEX mentors_decided_by_idx
    ON public.mentors (decided_by)
    WHERE decided_by IS NOT NULL;

-- mentor_availability — one table, two row shapes, exactly as cal.com models it.
--
-- A WEEKLY row names a weekday and recurs. A DATED row names one calendar date and
-- REPLACES that date's weekly rows rather than adding to them. The CHECK below enforces
-- that a row is one or the other and never both, which is why the Go type has two
-- constructors and no way to build the row this CHECK would reject.
--
-- WHY start_time AND end_time CARRY NO ZONE. They are wall-clock times, resolved through
-- mentors.timezone against a particular date. Storing an instant instead — or a zone per
-- row — breaks on the daylight-saving transition: "18:00 in Berlin" is a different UTC
-- instant in January and in July, and a mentor who said 18:00 means 18:00 in both. The
-- failure is silent, seasonal, and undiagnosable from the mentor's side.
--
-- WHY AN EMPTY SPAN IS LEGAL, BUT ONLY ON A DATED ROW. A dated row with equal start and
-- end CLOSES its date: it is how "I am away on the 16th" is one inserted row rather than
-- a deletion and re-creation of the weekly schedule. On a weekly row the same thing
-- states nothing at all, so it is refused. Note the Go engine goes one step beyond
-- cal.com here: a closure beats every other override on its date, because a slot wrongly
-- offered pulls a mentor out of a declared holiday while one wrongly withheld only costs
-- a booking.
--
-- 24:00:00 is a valid time in Postgres and is the intended way to say "until midnight"
-- without the row spilling onto the next date.
CREATE TABLE public.mentor_availability (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    mentor_id  bigint      NOT NULL REFERENCES public.mentors(id) ON DELETE CASCADE,
    -- A day of the week: 0 through 6, and a CHECK below says so. There will not be an
    -- eighth day.
    -- squawk-ignore prefer-bigint-over-smallint
    weekday    smallint,
    on_date    date,
    start_time time        NOT NULL,
    end_time   time        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    -- One shape or the other, never both and never neither.
    CONSTRAINT mentor_availability_shape_check
        CHECK ((weekday IS NULL) <> (on_date IS NULL)),
    CONSTRAINT mentor_availability_weekday_check
        CHECK (weekday IS NULL OR weekday BETWEEN 0 AND 6),
    -- A span never runs backwards. It may be empty only when dated, where emptiness is
    -- the point.
    CONSTRAINT mentor_availability_span_check
        CHECK (end_time >= start_time AND (on_date IS NOT NULL OR end_time > start_time))
);

COMMENT ON TABLE public.mentor_availability IS
    'A mentor''s availability in two row shapes: weekly (weekday set) and dated override '
    '(on_date set). A dated row replaces its whole date; an empty dated row closes it.';

COMMENT ON COLUMN public.mentor_availability.start_time IS
    'Wall-clock time with no zone and no date. Resolved through mentors.timezone, per '
    'date. Storing an instant here would move every slot by an hour for half the year.';

CREATE INDEX mentor_availability_mentor_idx ON public.mentor_availability (mentor_id);

-- mentor_bookings — one booked session.
--
-- WHY THE ID IS A RANDOM uuid. A booking is read by two different accounts, so a
-- countable id would make any single authorisation slip enumerable. Same reasoning, and
-- the same choice, as referral's ids (migration 0046).
--
-- WHY THERE IS NO 'completed' STATUS. Completion is not a decision anybody makes; it is
-- "confirmed, and the end has passed". A status column would need a worker to advance it
-- and would then be wrong for exactly as long as that worker was down.
--
-- meeting_url IS A SNAPSHOT. A mentor changing their link must not rewrite the invitation
-- somebody already has in their calendar.
--
-- seeker_timezone is what the seeker's own confirmation and reminders are rendered in.
-- The mentor's is on their profile; neither is derivable from the other.
CREATE TABLE public.mentor_bookings (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    mentor_id       bigint      NOT NULL REFERENCES public.mentors(id) ON DELETE CASCADE,
    seeker_user_id  bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    starts_at       timestamptz NOT NULL,
    ends_at         timestamptz NOT NULL,
    status          text        NOT NULL DEFAULT 'confirmed',
    job_id          bigint,
    note            text        NOT NULL DEFAULT '',
    seeker_timezone text        NOT NULL,
    meeting_url     text        NOT NULL,
    cancelled_at    timestamptz,
    cancelled_by    bigint      REFERENCES public.users(id) ON DELETE SET NULL,
    cancel_reason   text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT mentor_bookings_span_check   CHECK (ends_at > starts_at),
    CONSTRAINT mentor_bookings_status_check CHECK (status IN ('confirmed', 'cancelled')),
    -- A cancelled booking always records when, and a live one never does. Without this
    -- the two can drift, and "is this cancelled?" then has two answers.
    CONSTRAINT mentor_bookings_cancelled_check
        CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL)),

    -- THE NON-OVERLAP GUARANTEE. Two confirmed bookings for one mentor cannot share an
    -- instant, and the database is what says so — not a re-check in the service.
    --
    -- cal.com re-checks availability inside the booking transaction, which is a
    -- check-then-act race: two requests both pass the check, both insert, and one mentor
    -- has two people in the same hour. A constraint closes that window with no lock and
    -- no retry loop. The application still re-derives the slot before inserting, but for
    -- a different job: to give an ordinary refusal a REASON (paused mentor, elapsed
    -- notice, withdrawn day) instead of a constraint violation.
    --
    -- The bounds are half-open, '[)', matching Interval.Overlaps in
    -- internal/engage/mentorship: 18:00-19:00 and 19:00-20:00 are back-to-back, not a
    -- conflict. Go and Postgres must agree on this or a slot the engine offers is a slot
    -- the insert rejects.
    --
    -- The WHERE predicate is load-bearing: without it a cancelled 19:00 booking would
    -- block 19:00 forever.
    CONSTRAINT mentor_bookings_no_overlap
        EXCLUDE USING gist (
            mentor_id WITH =,
            tstzrange(starts_at, ends_at, '[)') WITH &&
        ) WHERE (status = 'confirmed')
);

COMMENT ON TABLE public.mentor_bookings IS
    'One booked mentorship session. Two confirmed rows for one mentor cannot overlap — '
    'the mentor_bookings_no_overlap EXCLUDE constraint, not the service, guarantees it.';

COMMENT ON COLUMN public.mentor_bookings.job_id IS
    'The vacancy the seeker came from, as context. ON DELETE SET NULL: the booking '
    'outlives the posting, the same seam referral_requests.job_id carries.';

-- The vacancy reference is added separately rather than inline because it locks a table
-- with ~11M rows: ADD CONSTRAINT ... FOREIGN KEY takes SHARE ROW EXCLUSIVE on the
-- REFERENCED table too, blocking writes to jobs while it is held.
--
-- Note what does NOT help: adding it NOT VALID. That skips the scan of the REFERENCING
-- table, which here is empty and free to scan anyway, and takes the same lock on jobs
-- either way. What actually bounds the risk is the runner's session-level
-- `SET lock_timeout = '5s'` — so a migration queued behind the nightly pg_dump FAILS
-- rather than holding the site's writes behind it — and deploying outside that dump
-- window. See internal/platform/migrate/migrate.go and the 2026-07-30 incident it
-- records.
ALTER TABLE public.mentor_bookings
    ADD CONSTRAINT mentor_bookings_job_id_fkey
    FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE SET NULL;

-- The mentor's own session list, and the busy set the slot engine subtracts.
CREATE INDEX mentor_bookings_mentor_starts_at_idx
    ON public.mentor_bookings (mentor_id, starts_at);

-- The seeker's session list, newest first.
CREATE INDEX mentor_bookings_seeker_starts_at_idx
    ON public.mentor_bookings (seeker_user_id, starts_at DESC);

-- The reminder worker's scan: confirmed sessions about to start. Partial, because a
-- cancelled booking is never reminded about.
CREATE INDEX mentor_bookings_upcoming_idx
    ON public.mentor_bookings (starts_at)
    WHERE status = 'confirmed';

-- The third user-referencing column, indexed for account deletion like the other two.
-- Partial for the same reason as mentors.decided_by: a live booking has nobody who
-- cancelled it, so most rows are NULL and none of them can match.
CREATE INDEX mentor_bookings_cancelled_by_idx
    ON public.mentor_bookings (cancelled_by)
    WHERE cancelled_by IS NOT NULL;

-- mentor_busy_intervals — the cache of a mentor's occupied time read from their own
-- calendar. CREATED EMPTY AND LEFT EMPTY by this change: the Google free/busy sync is a
-- separate follow-up, and only the seam belongs here.
--
-- IT HOLDS ONLY (start, end). No title, no attendee, no description, no link. This is the
-- same rule internal/application/calsync already lives by for candidates' calendars — the
-- window is read, used, and discarded, and the schema is what makes a mistake in the
-- reader impossible to persist. A calendar holds medical appointments, family, and
-- interviews with employers nobody told us about.
CREATE TABLE public.mentor_busy_intervals (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    mentor_id   bigint      NOT NULL REFERENCES public.mentors(id) ON DELETE CASCADE,
    starts_at   timestamptz NOT NULL,
    ends_at     timestamptz NOT NULL,
    source      text        NOT NULL,
    external_id text        NOT NULL,
    synced_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT mentor_busy_intervals_span_check CHECK (ends_at > starts_at),
    -- One row per calendar event per mentor, so a re-sync updates rather than duplicates.
    CONSTRAINT mentor_busy_intervals_event_key UNIQUE (mentor_id, source, external_id)
);

COMMENT ON TABLE public.mentor_busy_intervals IS
    'Occupied time read from a mentor''s own calendar, as bare intervals. Deliberately '
    'carries no title, attendee or link: the schema is what stops a reader mistake from '
    'persisting somebody''s dentist appointment. Empty until the calendar sync ships.';

CREATE INDEX mentor_busy_intervals_mentor_starts_at_idx
    ON public.mentor_busy_intervals (mentor_id, starts_at);

-- mentor_reviews — one review per completed session.
--
-- booking_id IS THE PRIMARY KEY, which is the whole "one review per booking, editable,
-- never duplicated" rule expressed as a constraint instead of as a service check. A
-- second submission is an upsert by construction.
CREATE TABLE public.mentor_reviews (
    booking_id     uuid        PRIMARY KEY REFERENCES public.mentor_bookings(id) ON DELETE CASCADE,
    mentor_id      bigint      NOT NULL REFERENCES public.mentors(id) ON DELETE CASCADE,
    seeker_user_id bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    -- One to five stars, and a CHECK below says so.
    -- squawk-ignore prefer-bigint-over-smallint
    rating         smallint    NOT NULL,
    comment        text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT mentor_reviews_rating_check CHECK (rating BETWEEN 1 AND 5)
);

COMMENT ON TABLE public.mentor_reviews IS
    'One review per completed session, keyed by the booking so a second submission is an '
    'update rather than a duplicate.';

-- The aggregate the public profile shows.
CREATE INDEX mentor_reviews_mentor_idx ON public.mentor_reviews (mentor_id);

-- Account deletion again. NOT partial, unlike the other two: every review has a seeker,
-- so a predicate would exclude nothing and only cost a reader the question of why it is
-- there.
CREATE INDEX mentor_reviews_seeker_idx ON public.mentor_reviews (seeker_user_id);

-- mentor_booking_reminders — what has already been sent, so cmd/mentorship-remind is
-- idempotent per (booking, offset).
--
-- The composite primary key IS the idempotency: a re-run inserts a duplicate key and
-- sends nothing. Without it, a worker that runs every few minutes sends "your session
-- starts in an hour" every few minutes.
--
-- offset_minutes records WHICH reminder, not when it was due. The offsets ship fixed at
-- 1440 and 60; storing the number rather than an enum means adding a third costs no
-- migration.
CREATE TABLE public.mentor_booking_reminders (
    booking_id     uuid        NOT NULL REFERENCES public.mentor_bookings(id) ON DELETE CASCADE,
    -- Minutes before the session: 1440 and 60 today. A label, not a counter.
    -- squawk-ignore prefer-bigint-over-int
    offset_minutes integer     NOT NULL,
    sent_at        timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (booking_id, offset_minutes),
    CONSTRAINT mentor_booking_reminders_offset_check CHECK (offset_minutes > 0)
);

COMMENT ON TABLE public.mentor_booking_reminders IS
    'One row per reminder actually sent. The composite key is the idempotency guard: '
    're-running the reminder worker inserts a duplicate key and sends nothing.';
