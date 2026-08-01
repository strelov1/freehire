-- When the interview is — the one date a job search produces that nothing here holds.
--
-- mailclassify has `interview_invitation`, but that signal says an invitation ARRIVED and
-- is dated by emails.received_at. The meeting's own time is in two places we do not read:
-- the text/calendar part the ATS attaches, and the candidate's calendar once they accept.
--
-- Applied to a fresh volume by initdb after 0069; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code that reads it. Additive, no backfill —
-- cmd/cal-sync populates it once a candidate grants the calendar scope, and does nothing
-- at all until one does.

-- The meeting identifier carried by an invitation's text/calendar part (internal/ical).
-- Empty for the mail that has none, which is most of it.
--
-- This is the hinge of the whole feature. The invitation is already tied to an
-- application by the deterministic matcher, and this UID is the same identifier the entry
-- in the candidate's calendar carries — so the two are provably one meeting, with nothing
-- inferred. It is the only correspondence allowed to link automatically; a company name in
-- a title or an organiser's domain produces a suggestion the candidate confirms.
ALTER TABLE public.emails
    ADD COLUMN IF NOT EXISTS ical_uid text NOT NULL DEFAULT '';

-- The lookup calmatch runs per calendar event. Partial: the column is empty on the large
-- majority of rows, and an index over those entries would be mostly dead weight.
CREATE INDEX IF NOT EXISTS emails_user_ical_uid_idx
    ON public.emails (user_id, ical_uid) WHERE ical_uid <> '';

-- Which Google scopes this grant actually carries.
--
-- The calendar consent is asked for separately, so a connection may hold mail only. The
-- worker reads this rather than assuming: a token minted before the calendar scope existed
-- cannot call the calendar API, and learning that from a 403 once per user per run is a
-- worse answer than not asking. The table keeps its name — it is now the Google grant row
-- and a candidate may hold a calendar without a mailbox, but renaming it would mean
-- editing the whole mail stack for a word.
ALTER TABLE public.gmail_connections
    ADD COLUMN IF NOT EXISTS scopes text[] NOT NULL DEFAULT '{}';

-- One row per meeting the sync could attach to an application.
--
-- Deliberately NOT a kind in application_events. That ledger is append-only, so a
-- reschedule could not be expressed; and its occurred_at means "when this happened", read
-- by every day calculation — a row dated in the future would turn the record of a search
-- into a schedule, and the tracking calendar would draw it as something already done. The
-- ledger instead records `interview_scheduled` at the moment the scheduling was observed,
-- which is a fact about the past and stays true after a cancellation.
CREATE TABLE IF NOT EXISTS application_interviews (
    id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- NOT NULL, and that is the privacy boundary rather than a note about one.
    --
    -- A calendar holds medical appointments, family, the current employer's meetings, and
    -- interviews with employers the candidate never told us about. The rule is that only a
    -- meeting attached to one of their applications is stored — and expressing it here
    -- means a bug in the worker cannot write a dentist appointment, because the column
    -- naming the application it belongs to cannot be empty.
    --
    -- ON DELETE CASCADE: untracking the application removes the interview with it. Unlike
    -- the ledger, this table is not a record of what happened — it is the appointment.
    application_id bigint      NOT NULL REFERENCES applications (id) ON DELETE CASCADE,
    -- The meeting's own identity, shared with the invitation that named it.
    ical_uid       text        NOT NULL,
    starts_at      timestamptz NOT NULL,
    ends_at        timestamptz,
    -- What the candidate needs in order to keep the appointment, and nothing else. No
    -- attendee list, no description, no organiser: those belong to the calendar, and every
    -- field here is one we would have to justify holding.
    title          text        NOT NULL DEFAULT '',
    join_url       text        NOT NULL DEFAULT '',
    -- confirmed | cancelled. A cancellation marks rather than deletes: an empty Thursday
    -- cannot be told apart from a calendar that failed to load.
    status         text        NOT NULL DEFAULT 'confirmed',
    -- Where the meeting was read from, in the manner of application_events.source. One
    -- value today; an ICS subscription would be a second without changing anything here.
    source         text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

-- Idempotency by constraint: the sync re-reads its whole window every run, so a meeting it
-- has already seen must update rather than accumulate. The key is (user, meeting), not
-- (user, application) — one application can hold several rounds.
CREATE UNIQUE INDEX IF NOT EXISTS application_interviews_user_uid_key
    ON application_interviews (user_id, ical_uid);

-- The calendar's range read, and the account-deletion cascade.
CREATE INDEX IF NOT EXISTS application_interviews_user_starts_idx
    ON application_interviews (user_id, starts_at);
