-- name: CreateMentorProfile :one
-- Submit a mentor profile. Starts pending, awaiting a human moderator — nothing else
-- makes it public, including an approved referral_offers row for the same company.
-- UNIQUE (user_id) rejects a second profile; the FK on company_slug rejects a company
-- the catalogue does not carry. The repository maps both violations to domain errors.
INSERT INTO mentors (
    user_id, company_slug, slug, headline, bio, topics, languages, timezone,
    session_duration_min, buffer_before_min, buffer_after_min, min_notice_min,
    horizon_days, meeting_url
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetMentorByUserID :one
-- The owner's own profile, whatever its status — the mentor cabinet reads this, and a
-- pending or rejected profile must still be visible to the person who submitted it.
SELECT * FROM mentors WHERE user_id = $1;

-- name: GetMentorByID :one
-- By primary key, for the paths that already hold one (booking, moderation).
SELECT * FROM mentors WHERE id = $1;

-- name: GetPublishedMentorBySlug :one
-- The PUBLIC profile read. Predicated rather than filtered in the service: a pending,
-- rejected or paused profile must answer as though it does not exist, and putting that
-- rule anywhere but the query leaves a second reader free to forget it.
-- sqlc.embed keeps the mentor row as one db.Mentor instead of forty loose columns, so
-- the adapter maps it once rather than re-assembling it per query.
SELECT sqlc.embed(m), c.name AS company_name
FROM mentors m
LEFT JOIN companies c ON c.slug = m.company_slug
WHERE m.slug = $1 AND m.status = 'approved' AND NOT m.paused;

-- name: UpdateMentorProfile :one
-- The mentor edits their own profile. The user_id guard scopes it to the owner, so a
-- foreign id updates zero rows and the repository reports "not found" rather than
-- revealing that the profile exists. Editing does NOT reset moderation: a mentor
-- rewording their headline should not vanish from the directory for a day.
UPDATE mentors
SET headline = sqlc.arg(headline),
    bio = sqlc.arg(bio),
    topics = sqlc.arg(topics),
    languages = sqlc.arg(languages),
    timezone = sqlc.arg(timezone),
    session_duration_min = sqlc.arg(session_duration_min),
    buffer_before_min = sqlc.arg(buffer_before_min),
    buffer_after_min = sqlc.arg(buffer_after_min),
    min_notice_min = sqlc.arg(min_notice_min),
    horizon_days = sqlc.arg(horizon_days),
    meeting_url = sqlc.arg(meeting_url),
    updated_at = now()
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id)
RETURNING *;

-- name: SetMentorPaused :one
-- The mentor's own switch. Deliberately independent of status: pausing and resuming
-- need no moderator, and neither may alter what the moderator decided.
UPDATE mentors
SET paused = sqlc.arg(paused), updated_at = now()
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id)
RETURNING *;

-- name: DecideMentorProfile :one
-- Approve or reject a pending profile, recording the moderator and the time. The
-- status='pending' guard makes a second decision match no row, which the repository
-- maps to "not pending" — the same shape DecideReferralOffer uses.
UPDATE mentors
SET status = sqlc.arg(status), decided_by = sqlc.arg(decided_by),
    decided_at = now(), updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: ListPendingMentorProfiles :many
-- The moderation queue, oldest first. Carries whether the account also holds an APPROVED
-- referral offer for the same company — corroborating evidence for the human deciding,
-- never a gate: nothing in this change approves a profile automatically.
-- Capped at 500 for the same reason the referral queue is: a backlog deeper than that
-- needs triage, not a longer page.
SELECT sqlc.embed(m), c.name AS company_name,
    EXISTS (
        SELECT 1 FROM referral_offers r
        WHERE r.user_id = m.user_id AND r.company_slug = m.company_slug
          AND r.status = 'approved'
    ) AS has_approved_referral_offer
FROM mentors m
LEFT JOIN companies c ON c.slug = m.company_slug
WHERE m.status = 'pending'
ORDER BY m.created_at
LIMIT 500;

-- name: ListPublishedMentors :many
-- The public directory. Every filter is optional and applied as "NULL means unfiltered",
-- which keeps one query instead of a builder; the endpoint reports any parameter it did
-- NOT read in meta.ignored_params, so a filter that vanishes from this list must vanish
-- from that vocabulary too.
-- Keyset pagination on (created_at, id) rather than OFFSET: the directory is browsed and
-- an OFFSET page silently repeats or skips a row when a profile is approved mid-browse.
SELECT sqlc.embed(m), c.name AS company_name,
    COALESCE(r.rating_count, 0)::bigint AS rating_count,
    COALESCE(r.rating_avg, 0)::numeric  AS rating_avg
FROM mentors m
LEFT JOIN companies c ON c.slug = m.company_slug
LEFT JOIN (
    SELECT mentor_id, count(*) AS rating_count, avg(rating) AS rating_avg
    FROM mentor_reviews GROUP BY mentor_id
) r ON r.mentor_id = m.id
WHERE m.status = 'approved' AND NOT m.paused
  AND (sqlc.narg(company_slug)::text IS NULL OR m.company_slug = sqlc.narg(company_slug)::text)
  AND (sqlc.narg(topic)::text IS NULL OR sqlc.narg(topic)::text = ANY (m.topics))
  AND (sqlc.narg(language)::text IS NULL OR sqlc.narg(language)::text = ANY (m.languages))
  AND (
      sqlc.narg(before_created_at)::timestamptz IS NULL
      OR (m.created_at, m.id) < (sqlc.narg(before_created_at)::timestamptz, sqlc.narg(before_id)::bigint)
  )
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(row_limit);

-- name: DeleteMentorProfile :execrows
-- Withdrawal. The owner guard scopes it to the caller. Future bookings must be cancelled
-- and their seekers notified BEFORE this runs — the ON DELETE CASCADE would otherwise
-- take the booking rows with it and nobody would ever be told.
DELETE FROM mentors WHERE id = $1 AND user_id = $2;

-- name: ListMentorAvailability :many
-- Every availability row for a mentor, both shapes. The slot engine wants all of them at
-- once — an override only means anything beside the weekly rules it replaces — so this
-- deliberately does not filter by window.
SELECT * FROM mentor_availability WHERE mentor_id = $1 ORDER BY id;

-- name: CreateMentorAvailabilityRule :one
-- One availability row, weekly or dated. Which shape it is comes from which of weekday
-- and on_date is non-NULL; the mentor_availability_shape_check CHECK rejects a row that
-- sets both or neither, and the Go type cannot construct one either way.
INSERT INTO mentor_availability (mentor_id, weekday, on_date, start_time, end_time)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeleteMentorAvailabilityRule :execrows
-- The mentor_id guard scopes the delete to the owner's own schedule.
DELETE FROM mentor_availability WHERE id = $1 AND mentor_id = $2;

-- name: DeleteMentorWeeklyAvailability :execrows
-- Clears the recurring week, leaving dated overrides alone. The cabinet edits the week as
-- a whole — a schedule is a shape, not a list of rows a user reasons about individually —
-- so a save is this followed by inserts, inside one transaction.
DELETE FROM mentor_availability WHERE mentor_id = $1 AND weekday IS NOT NULL;

-- name: CreateMentorBooking :one
-- Book a session. The mentor_bookings_no_overlap EXCLUDE constraint is what guarantees
-- two confirmed bookings cannot share an instant, so a lost race raises a constraint
-- violation here; the repository maps it to the SAME "no longer available" error a stale
-- page gets, because to the client a race and a stale tab are the same event.
INSERT INTO mentor_bookings (
    mentor_id, seeker_user_id, starts_at, ends_at, job_id, note, seeker_timezone, meeting_url
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetMentorBooking :one
-- One booking with what both parties' views need. Authorisation is the caller's job: this
-- returns the row for any id, and every caller must check the reader is its mentor or its
-- seeker before rendering it.
SELECT sqlc.embed(b), m.slug AS mentor_slug, m.user_id AS mentor_user_id,
       m.timezone AS mentor_timezone, m.company_slug, m.headline,
       mu.email AS mentor_email, su.email AS seeker_email
FROM mentor_bookings b
JOIN mentors m ON m.id = b.mentor_id
JOIN users mu ON mu.id = m.user_id
JOIN users su ON su.id = b.seeker_user_id
WHERE b.id = $1;

-- name: ListMentorBusyBookings :many
-- The busy set the slot engine subtracts: this mentor's CONFIRMED bookings overlapping
-- the window. Buffers are NOT applied here — the engine widens these, because the buffers
-- are the mentor's and may change between two reads of the same booking.
-- Half-open on both sides, matching the EXCLUDE constraint and Interval.Overlaps: a
-- booking that merely abuts the window is not in it.
SELECT starts_at, ends_at
FROM mentor_bookings
WHERE mentor_id = $1 AND status = 'confirmed'
  AND starts_at < sqlc.arg(window_end) AND ends_at > sqlc.arg(window_start)
ORDER BY starts_at;

-- name: ListMentorBusyIntervals :many
-- The other half of the busy set: intervals read from the mentor's own calendar. Empty
-- until that sync ships, and carries only the bounds — no title, no attendee.
SELECT starts_at, ends_at
FROM mentor_busy_intervals
WHERE mentor_id = $1
  AND starts_at < sqlc.arg(window_end) AND ends_at > sqlc.arg(window_start)
ORDER BY starts_at;

-- name: ListBookingsBySeeker :many
-- The seeker's own sessions, newest first. Upcoming and past are split by the caller
-- against one clock rather than by two queries against two.
SELECT sqlc.embed(b), m.slug AS mentor_slug, m.headline, m.company_slug,
       c.name AS company_name
FROM mentor_bookings b
JOIN mentors m ON m.id = b.mentor_id
LEFT JOIN companies c ON c.slug = m.company_slug
WHERE b.seeker_user_id = $1
ORDER BY b.starts_at DESC
LIMIT sqlc.arg(row_limit);

-- name: ListBookingsByMentor :many
-- The mentor's own sessions. Carries the seeker's email so the cabinet can show who is
-- coming; the mentor is meeting this person, so their identity is not a leak.
SELECT sqlc.embed(b), u.email AS seeker_email
FROM mentor_bookings b
JOIN users u ON u.id = b.seeker_user_id
WHERE b.mentor_id = $1
ORDER BY b.starts_at DESC
LIMIT sqlc.arg(row_limit);

-- name: CancelMentorBooking :one
-- Cancel before the session starts. Three guards in one statement, all load-bearing:
-- status='confirmed' makes a second cancellation match no row; starts_at > now() refuses
-- a session already begun; and cancelled_by must be the mentor's account or the seeker's,
-- which is what stops a stranger cancelling somebody else's meeting. A no-row result is
-- reported as "not found" without saying which guard failed — a stranger must not learn
-- that the booking exists.
UPDATE mentor_bookings b
SET status = 'cancelled', cancelled_at = now(),
    -- Cast so the parameter is a plain id: the COLUMN is nullable (a live booking has
    -- nobody who cancelled it), but the person doing the cancelling is always known.
    cancelled_by = sqlc.arg(cancelled_by)::bigint, cancel_reason = sqlc.arg(cancel_reason)
FROM mentors m
WHERE b.mentor_id = m.id
  AND b.id = sqlc.arg(id)
  AND b.status = 'confirmed'
  AND b.starts_at > now()
  AND sqlc.arg(cancelled_by)::bigint IN (b.seeker_user_id, m.user_id)
RETURNING b.*;

-- name: CancelFutureBookingsForMentor :many
-- Withdrawal and any other wholesale removal: cancel every future confirmed session at
-- once, RETURNING enough to notify each seeker. Their notifications are sent after this
-- commits — a booking cancelled without its seeker being told is worse than one not
-- cancelled.
UPDATE mentor_bookings b
SET status = 'cancelled', cancelled_at = now(),
    -- Cast so the parameter is a plain id: the COLUMN is nullable (a live booking has
    -- nobody who cancelled it), but the person doing the cancelling is always known.
    cancelled_by = sqlc.arg(cancelled_by)::bigint, cancel_reason = sqlc.arg(cancel_reason)
FROM users u
WHERE u.id = b.seeker_user_id
  AND b.mentor_id = sqlc.arg(mentor_id) AND b.status = 'confirmed' AND b.starts_at > now()
RETURNING sqlc.embed(b), u.email AS seeker_email;

-- name: ListBookingsDueForReminder :many
-- The reminder worker's page: confirmed sessions starting within the offset and not yet
-- reminded at it.
-- starts_at > now() is what stops a late run firing "your session starts in an hour"
-- after the session — silence is better than that message arriving afterwards.
-- The NOT EXISTS makes the read idempotent alongside the insert below; the composite key
-- makes the WRITE idempotent, and both are needed because two runs can overlap.
SELECT sqlc.embed(b), m.timezone AS mentor_timezone, m.user_id AS mentor_user_id,
       m.slug AS mentor_slug, m.headline, m.meeting_url AS mentor_meeting_url,
       u.email AS seeker_email, mu.email AS mentor_email
FROM mentor_bookings b
JOIN mentors m ON m.id = b.mentor_id
JOIN users u ON u.id = b.seeker_user_id
JOIN users mu ON mu.id = m.user_id
WHERE b.status = 'confirmed'
  AND b.starts_at > now()
  AND b.starts_at <= now() + make_interval(mins => sqlc.arg(offset_minutes)::int)
  AND NOT EXISTS (
      SELECT 1 FROM mentor_booking_reminders r
      WHERE r.booking_id = b.id AND r.offset_minutes = sqlc.arg(offset_minutes)::int
  )
ORDER BY b.starts_at
LIMIT sqlc.arg(row_limit);

-- name: RecordReminderSent :execrows
-- Claim one reminder. ON CONFLICT DO NOTHING means a concurrent run inserts nothing and
-- gets zero rows back — so this is the CLAIM, and a caller that sends before checking the
-- row count sends twice. Send after this returns 1, never before.
INSERT INTO mentor_booking_reminders (booking_id, offset_minutes)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: UpsertMentorReview :one
-- One review per booking, editable. booking_id is the primary key, so a second submission
-- is an update by construction rather than by a service check — which is also why the
-- review count cannot drift from the number of reviewed sessions.
INSERT INTO mentor_reviews (booking_id, mentor_id, seeker_user_id, rating, comment)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (booking_id) DO UPDATE
SET rating = EXCLUDED.rating, comment = EXCLUDED.comment, updated_at = now()
RETURNING *;

-- name: GetMentorReviewSummary :one
-- The aggregate the public profile shows. Returns zeroes rather than NULLs for a mentor
-- nobody has reviewed, so the caller renders "no reviews yet" from a count of 0 instead
-- of from a null it has to remember to check.
SELECT COALESCE(count(*), 0)::bigint AS rating_count,
       COALESCE(avg(rating), 0)::numeric AS rating_avg
FROM mentor_reviews WHERE mentor_id = $1;

-- name: GetMentorReviewForBooking :one
-- The seeker's own review of one session, for rendering the edit form.
SELECT * FROM mentor_reviews WHERE booking_id = $1;
