-- name: GetBuddyMockSettings :one
-- A member's mock-host settings, or no row when they never saved any.
SELECT * FROM buddy_mock_settings WHERE user_id = $1;

-- name: UpsertBuddyMockSettings :one
-- Save the whole settings block. The service validates zone, link and topics first; the
-- CHECKs repeat the link and topic rules so no other writer can skip them.
INSERT INTO buddy_mock_settings (user_id, timezone, meeting_url, topics, paused)
VALUES (sqlc.arg(user_id), sqlc.arg(timezone), sqlc.arg(meeting_url), sqlc.arg(topics)::text[], sqlc.arg(paused))
ON CONFLICT (user_id) DO UPDATE
SET timezone = EXCLUDED.timezone, meeting_url = EXCLUDED.meeting_url, topics = EXCLUDED.topics,
    paused = EXCLUDED.paused, updated_at = now()
RETURNING *;

-- name: ListBuddyMockAvailability :many
-- Every availability row of a member, both shapes: an override only means anything beside
-- the weekly rules it replaces.
SELECT * FROM buddy_mock_availability WHERE user_id = $1 ORDER BY id;

-- name: ReplaceBuddyMockWeekly :exec
-- Swap the recurring week in one statement, leaving dated overrides alone: half of the
-- edit applied is a member bookable at hours they just removed. The three arrays are one
-- row each, zipped back by position (WITH ORDINALITY: sqlc's analyzer does not resolve the
-- multi-array form of unnest).
WITH cleared AS (
    DELETE FROM buddy_mock_availability
    WHERE user_id = sqlc.arg(user_id)::bigint AND weekday IS NOT NULL
)
INSERT INTO buddy_mock_availability (user_id, weekday, start_time, end_time)
SELECT sqlc.arg(user_id)::bigint, w.weekday, s.start_time, e.end_time
FROM unnest(sqlc.arg(weekdays)::smallint[]) WITH ORDINALITY AS w (weekday, ord)
JOIN unnest(sqlc.arg(start_times)::time[]) WITH ORDINALITY AS s (start_time, ord) ON s.ord = w.ord
JOIN unnest(sqlc.arg(end_times)::time[]) WITH ORDINALITY AS e (end_time, ord) ON e.ord = w.ord;

-- name: CreateBuddyMockAvailabilityRule :one
-- One row, weekly or dated; which one is whichever of weekday and on_date is set.
INSERT INTO buddy_mock_availability (user_id, weekday, on_date, start_time, end_time)
VALUES (sqlc.arg(user_id), sqlc.narg(weekday), sqlc.narg(on_date), sqlc.arg(start_time), sqlc.arg(end_time))
RETURNING *;

-- name: DeleteBuddyMockAvailabilityRule :execrows
-- The user_id guard scopes the delete to the owner's own schedule.
DELETE FROM buddy_mock_availability WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: ListBuddyMockBusy :many
-- A member's occupied time in a window: every mock they are in, as host or as guest. The
-- slot engine subtracts it from their availability.
SELECT starts_at, ends_at
FROM buddy_mock_participants
WHERE user_id = sqlc.arg(user_id)
  AND active
  AND starts_at < sqlc.arg(window_end)
  AND ends_at > sqlc.arg(window_start)
ORDER BY starts_at;

-- name: CountFutureBuddyMocksAsGuest :one
-- The spam bound's count: confirmed mocks the member is a guest in that have not started.
SELECT count(*) FROM buddy_mocks
WHERE guest_user_id = $1 AND status = 'confirmed' AND starts_at > now();

-- name: CreateBuddyMock :one
-- Book a mock and both participant rows in ONE statement. The guard repeats, inside the
-- write, what the service read before it: both people are active members and no block
-- stands between them; a pair that cannot meet inserts nothing and returns no row. The
-- buddy_mock_participants_no_overlap EXCLUDE refuses a person already in an overlapping
-- mock in either role; the repository maps that 23P01 to the same "slot unavailable" a
-- stale page gets.
WITH mock AS (
    INSERT INTO buddy_mocks (host_user_id, guest_user_id, starts_at, ends_at, topic, guest_timezone)
    SELECT sqlc.arg(host_user_id)::bigint, sqlc.arg(guest_user_id)::bigint,
           sqlc.arg(starts_at)::timestamptz, sqlc.arg(ends_at)::timestamptz,
           sqlc.arg(topic)::text, sqlc.arg(guest_timezone)::text
    WHERE EXISTS (SELECT 1 FROM buddy_members
                  WHERE user_id = sqlc.arg(host_user_id)::bigint AND left_at IS NULL)
      AND EXISTS (SELECT 1 FROM buddy_members
                  WHERE user_id = sqlc.arg(guest_user_id)::bigint AND left_at IS NULL)
      AND NOT EXISTS (
          SELECT 1 FROM user_blocks b
          WHERE (b.blocker_id = sqlc.arg(host_user_id)::bigint AND b.blocked_id = sqlc.arg(guest_user_id)::bigint)
             OR (b.blocker_id = sqlc.arg(guest_user_id)::bigint AND b.blocked_id = sqlc.arg(host_user_id)::bigint))
    RETURNING id, host_user_id, guest_user_id, starts_at, ends_at
), seats AS (
    INSERT INTO buddy_mock_participants (mock_id, user_id, starts_at, ends_at)
    SELECT mock.id, mock.host_user_id, mock.starts_at, mock.ends_at FROM mock
    UNION ALL
    SELECT mock.id, mock.guest_user_id, mock.starts_at, mock.ends_at FROM mock
)
SELECT id FROM mock;

-- name: GetBuddyMock :one
-- One mock with what both participants' notices need: both handles, the host's CURRENT
-- zone and link, and both addresses — each address only ever addresses its own owner's
-- notice. Authorisation is the caller's: this answers for any id.
SELECT m.id, m.host_user_id, m.guest_user_id, m.starts_at, m.ends_at, m.topic, m.status,
       m.guest_timezone, m.cancel_reason,
       hm.handle AS host_handle, gm.handle AS guest_handle,
       coalesce(s.timezone, '')::text AS host_timezone,
       coalesce(s.meeting_url, '')::text AS meeting_url,
       hu.email AS host_email, gu.email AS guest_email
FROM buddy_mocks m
JOIN buddy_members hm ON hm.user_id = m.host_user_id
JOIN buddy_members gm ON gm.user_id = m.guest_user_id
JOIN users hu ON hu.id = m.host_user_id
JOIN users gu ON gu.id = m.guest_user_id
LEFT JOIN buddy_mock_settings s ON s.user_id = m.host_user_id
WHERE m.id = $1;

-- name: ListBuddyMocksForUser :many
-- A member's own mocks, newest first, with the host's current link, whether the other side
-- may still be drawn (both active, no block either way — the ListBuddyCounterparts rule),
-- the member's own review, and the review ABOUT them only once revealed: they have written
-- theirs, or seven days have passed since the end. The same reveal rule is restated in
-- GetBuddyMemberMockSummary and ListBuddyMemberRevealedReviews; change all three together.
SELECT m.id, m.host_user_id, m.guest_user_id, m.starts_at, m.ends_at, m.topic, m.status,
       m.cancel_reason,
       hm.handle AS host_handle, gm.handle AS guest_handle,
       s.meeting_url AS host_meeting_url,
       (cm.left_at IS NULL
        AND NOT EXISTS (
            SELECT 1 FROM user_blocks b
            WHERE (b.blocker_id = sqlc.arg(user_id)::bigint AND b.blocked_id = cm.user_id)
               OR (b.blocker_id = cm.user_id AND b.blocked_id = sqlc.arg(user_id)::bigint)))::boolean AS counterpart_visible,
       mine.rating AS my_rating, mine.comment AS my_comment,
       theirs.rating AS their_rating, theirs.comment AS their_comment
FROM buddy_mocks m
JOIN buddy_members hm ON hm.user_id = m.host_user_id
JOIN buddy_members gm ON gm.user_id = m.guest_user_id
JOIN buddy_members cm ON cm.user_id = CASE WHEN m.host_user_id = sqlc.arg(user_id)::bigint
                                           THEN m.guest_user_id ELSE m.host_user_id END
LEFT JOIN buddy_mock_settings s ON s.user_id = m.host_user_id
LEFT JOIN buddy_mock_reviews mine
       ON mine.mock_id = m.id AND mine.author_user_id = sqlc.arg(user_id)::bigint
LEFT JOIN buddy_mock_reviews theirs
       ON theirs.mock_id = m.id AND theirs.subject_user_id = sqlc.arg(user_id)::bigint
      AND (mine.mock_id IS NOT NULL OR m.ends_at + interval '7 days' <= now())
WHERE m.host_user_id = sqlc.arg(user_id)::bigint OR m.guest_user_id = sqlc.arg(user_id)::bigint
ORDER BY m.starts_at DESC
LIMIT sqlc.arg(row_limit);

-- name: CancelBuddyMock :one
-- A participant cancels a confirmed mock before it starts, freeing both people's time in
-- the same statement. No row: not a participant, already cancelled, or already started.
WITH cancelled AS (
    UPDATE buddy_mocks m
    SET status = 'cancelled', cancelled_at = now(), cancelled_by = sqlc.arg(user_id)::bigint,
        cancel_reason = 'participant'
    WHERE m.id = sqlc.arg(id)
      AND m.status = 'confirmed'
      AND m.starts_at > now()
      AND (m.host_user_id = sqlc.arg(user_id)::bigint OR m.guest_user_id = sqlc.arg(user_id)::bigint)
    RETURNING m.id
), freed AS (
    UPDATE buddy_mock_participants p
    SET active = false
    WHERE p.mock_id IN (SELECT id FROM cancelled)
)
SELECT id FROM cancelled;

-- name: ClaimBuddyMockCancelNotices :many
-- Claim the cancellations an actor caused that nobody has announced yet — by their own
-- cancel, their leave, or their block (whose trigger cannot return rows). A concurrent
-- claim waits on the row lock and then matches nothing, so each notice goes out once.
-- Bounded to future mocks cancelled in the last day: an announcement lost to a crash is not
-- worth sending weeks later.
UPDATE buddy_mocks
SET cancel_announced_at = now()
WHERE cancelled_by = sqlc.arg(actor_id)::bigint
  AND status = 'cancelled'
  AND cancel_announced_at IS NULL
  AND cancelled_at > now() - interval '1 day'
  AND starts_at > now()
RETURNING id;

-- name: CreateBuddyMockReview :execrows
-- One review of the other participant, once, between the mock's end and seven days after
-- it, and only of a confirmed mock. Zero rows: already reviewed (the key) or outside the
-- rules (the WHERE); the service reads the mock first to say which.
INSERT INTO buddy_mock_reviews (mock_id, author_user_id, subject_user_id, rating, comment)
SELECT m.id, sqlc.arg(author_user_id)::bigint,
       CASE WHEN m.host_user_id = sqlc.arg(author_user_id)::bigint THEN m.guest_user_id ELSE m.host_user_id END,
       sqlc.arg(rating)::smallint, sqlc.arg(comment)::text
FROM buddy_mocks m
WHERE m.id = sqlc.arg(mock_id)
  AND m.status = 'confirmed'
  AND m.ends_at <= now()
  AND m.ends_at + interval '7 days' > now()
  AND (m.host_user_id = sqlc.arg(author_user_id)::bigint OR m.guest_user_id = sqlc.arg(author_user_id)::bigint)
ON CONFLICT (mock_id, author_user_id) DO NOTHING;

-- name: GetBuddyMemberMockSummary :one
-- What a member page shows: completed mocks (confirmed and ended, either role) and the
-- count and average of REVEALED reviews about the member. Zeroes, never NULLs.
SELECT
    (SELECT count(*) FROM buddy_mocks c
     WHERE c.status = 'confirmed' AND c.ends_at <= now()
       AND (c.host_user_id = sqlc.arg(subject_user_id)::bigint
            OR c.guest_user_id = sqlc.arg(subject_user_id)::bigint))::bigint AS completed,
    count(r.mock_id)::bigint AS rating_count,
    coalesce(avg(r.rating), 0)::float8 AS rating_avg
FROM buddy_mock_reviews r
JOIN buddy_mocks m ON m.id = r.mock_id
WHERE r.subject_user_id = sqlc.arg(subject_user_id)::bigint
  AND (EXISTS (SELECT 1 FROM buddy_mock_reviews o
               WHERE o.mock_id = r.mock_id AND o.author_user_id = r.subject_user_id)
       OR m.ends_at + interval '7 days' <= now());

-- name: ListBuddyMemberRevealedReviews :many
-- The most recent revealed reviews about a member, without their authors: who mocked with
-- whom is a social graph no member page needs to publish.
SELECT r.rating, r.comment, r.created_at, m.topic
FROM buddy_mock_reviews r
JOIN buddy_mocks m ON m.id = r.mock_id
WHERE r.subject_user_id = sqlc.arg(subject_user_id)::bigint
  AND (EXISTS (SELECT 1 FROM buddy_mock_reviews o
               WHERE o.mock_id = r.mock_id AND o.author_user_id = r.subject_user_id)
       OR m.ends_at + interval '7 days' <= now())
ORDER BY r.created_at DESC
LIMIT sqlc.arg(row_limit);

-- name: ListBuddyMocksDueForReminder :many
-- Confirmed mocks starting inside one offset's window (floor, offset] and not yet reminded
-- at it. starts_at > now() keeps a late run from reminding about a mock already under way.
SELECT m.id
FROM buddy_mocks m
WHERE m.status = 'confirmed'
  AND m.starts_at > now()
  AND m.starts_at <= now() + make_interval(mins => sqlc.arg(offset_minutes)::int)
  AND m.starts_at > now() + make_interval(mins => sqlc.arg(floor_minutes)::int)
  AND NOT EXISTS (
      SELECT 1 FROM buddy_mock_reminders r
      WHERE r.mock_id = m.id AND r.offset_minutes = sqlc.arg(offset_minutes)::int)
ORDER BY m.starts_at
LIMIT sqlc.arg(row_limit);

-- name: RecordBuddyMockReminder :execrows
-- Claim one reminder. Zero rows means another run holds it, or the mock was cancelled or
-- has started since the due read: send only after this returns 1. The status is checked
-- here, in the claim, because a batch leaves time between that read and this statement.
INSERT INTO buddy_mock_reminders (mock_id, offset_minutes)
SELECT m.id, sqlc.arg(offset_minutes)::int
FROM buddy_mocks m
WHERE m.id = sqlc.arg(mock_id) AND m.status = 'confirmed' AND m.starts_at > now()
ON CONFLICT DO NOTHING;

-- name: DeleteBuddyMockReminder :execrows
-- Give a claim back after a failed delivery, so the next run retries.
DELETE FROM buddy_mock_reminders
WHERE mock_id = sqlc.arg(mock_id) AND offset_minutes = sqlc.arg(offset_minutes);
