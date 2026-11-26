-- name: ListUserAvailability :many
-- Every availability row of a user, both shapes: an override only means anything beside the
-- weekly rules it replaces.
SELECT * FROM user_availability WHERE user_id = $1 ORDER BY id;

-- name: ListUserAvailabilityForUsers :many
SELECT * FROM user_availability
WHERE user_id = ANY (sqlc.arg(user_ids)::bigint[])
ORDER BY user_id, id;

-- name: ReplaceUserAvailabilityWeekly :exec
-- Swap the recurring week in one statement, leaving dated overrides alone: half of the
-- edit applied is a user bookable at hours they just removed. The three arrays are one row
-- each, zipped back by position (WITH ORDINALITY: sqlc's analyzer does not resolve the
-- multi-array form of unnest).
WITH cleared AS (
    DELETE FROM user_availability
    WHERE user_id = sqlc.arg(user_id)::bigint AND weekday IS NOT NULL
)
INSERT INTO user_availability (user_id, weekday, start_time, end_time)
SELECT sqlc.arg(user_id)::bigint, w.weekday, s.start_time, e.end_time
FROM unnest(sqlc.arg(weekdays)::smallint[]) WITH ORDINALITY AS w (weekday, ord)
JOIN unnest(sqlc.arg(start_times)::time[]) WITH ORDINALITY AS s (start_time, ord) ON s.ord = w.ord
JOIN unnest(sqlc.arg(end_times)::time[]) WITH ORDINALITY AS e (end_time, ord) ON e.ord = w.ord;

-- name: CreateUserAvailabilityRule :one
-- One row, weekly or dated; which one is whichever of weekday and on_date is set.
INSERT INTO user_availability (user_id, weekday, on_date, start_time, end_time)
VALUES (sqlc.arg(user_id), sqlc.narg(weekday), sqlc.narg(on_date), sqlc.arg(start_time), sqlc.arg(end_time))
RETURNING *;

-- name: DeleteUserAvailabilityRule :execrows
-- The user_id guard scopes the delete to the owner's own schedule.
DELETE FROM user_availability WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: GetUserAvailabilitySettings :one
SELECT * FROM user_availability_settings WHERE user_id = $1;

-- name: ListUserAvailabilitySettings :many
SELECT * FROM user_availability_settings WHERE user_id = ANY (sqlc.arg(user_ids)::bigint[]);

-- name: UpsertUserAvailabilitySettings :one
INSERT INTO user_availability_settings (user_id, timezone, booking_duration_min, booking_notice_min)
VALUES (sqlc.arg(user_id), sqlc.arg(timezone), sqlc.arg(booking_duration_min), sqlc.arg(booking_notice_min))
ON CONFLICT (user_id) DO UPDATE
SET timezone = EXCLUDED.timezone, booking_duration_min = EXCLUDED.booking_duration_min,
    booking_notice_min = EXCLUDED.booking_notice_min, updated_at = now()
RETURNING *;

-- name: ListUserBusy :many
-- The occupied time of users in a window, from every product, so a meeting in one blocks
-- the same time in the others. source says where it came from; buffers are the engine's
-- business, not this query's. Half-open on both sides, like the EXCLUDE constraints.
--   mentor          a confirmed mentor session the user gives
--   mentor_seeker   a confirmed mentor session the user takes
--   buddy_mock      an active mock, host or guest
--   booking_link    a confirmed booking-link meeting
--   interview       any other confirmed application interview (no end = 30 minutes)
--   google          the synced calendar of the user's mentor profile
SELECT m.user_id, 'mentor'::text AS source, b.starts_at, b.ends_at
  FROM mentor_bookings b JOIN mentors m ON m.id = b.mentor_id
 WHERE m.user_id = ANY (sqlc.arg(user_ids)::bigint[]) AND b.status = 'confirmed'
   AND b.starts_at < sqlc.arg(window_end) AND b.ends_at > sqlc.arg(window_start)
UNION ALL
SELECT b.seeker_user_id, 'mentor_seeker', b.starts_at, b.ends_at
  FROM mentor_bookings b
 WHERE b.seeker_user_id = ANY (sqlc.arg(user_ids)::bigint[]) AND b.status = 'confirmed'
   AND b.starts_at < sqlc.arg(window_end) AND b.ends_at > sqlc.arg(window_start)
UNION ALL
SELECT m.user_id, 'google', i.starts_at, i.ends_at
  FROM mentor_busy_intervals i JOIN mentors m ON m.id = i.mentor_id
 WHERE m.user_id = ANY (sqlc.arg(user_ids)::bigint[])
   AND i.starts_at < sqlc.arg(window_end) AND i.ends_at > sqlc.arg(window_start)
UNION ALL
SELECT p.user_id, 'buddy_mock', p.starts_at, p.ends_at
  FROM buddy_mock_participants p
 WHERE p.user_id = ANY (sqlc.arg(user_ids)::bigint[]) AND p.active
   AND p.starts_at < sqlc.arg(window_end) AND p.ends_at > sqlc.arg(window_start)
UNION ALL
SELECT k.user_id, 'booking_link', k.starts_at, k.ends_at
  FROM booking_link_bookings k
 WHERE k.user_id = ANY (sqlc.arg(user_ids)::bigint[]) AND k.status = 'confirmed'
   AND k.starts_at < sqlc.arg(window_end) AND k.ends_at > sqlc.arg(window_start)
UNION ALL
-- Interviews a booking link wrote are skipped: they are counted once, as bookings.
SELECT i.user_id, 'interview', i.starts_at, coalesce(i.ends_at, i.starts_at + interval '30 minutes')
  FROM application_interviews i
 WHERE i.user_id = ANY (sqlc.arg(user_ids)::bigint[]) AND i.status = 'confirmed'
   AND i.source <> 'booking_link'
   AND i.starts_at < sqlc.arg(window_end)
   AND coalesce(i.ends_at, i.starts_at + interval '30 minutes') > sqlc.arg(window_start)
ORDER BY 1, 3;

-- name: SetUserAvailabilityZone :exec
-- A product's zone edit follows into the shared settings, which would otherwise keep
-- driving slots with the old zone. No row, nothing to follow.
UPDATE user_availability_settings SET timezone = sqlc.arg(timezone), updated_at = now()
 WHERE user_id = sqlc.arg(user_id);

-- name: EnsureUserAvailabilityZoneFromMentor :exec
-- Hours written through a product are stated in that product's zone; without a settings row
-- a booking link would read them in its own. Keeps an existing row.
INSERT INTO user_availability_settings (user_id, timezone)
SELECT m.user_id, m.timezone FROM mentors m WHERE m.id = $1
ON CONFLICT (user_id) DO NOTHING;

-- name: EnsureUserAvailabilityZoneFromBuddy :exec
INSERT INTO user_availability_settings (user_id, timezone)
SELECT s.user_id, s.timezone FROM buddy_mock_settings s WHERE s.user_id = $1
ON CONFLICT (user_id) DO NOTHING;

-- name: UpsertUserAvailabilityZone :exec
INSERT INTO user_availability_settings (user_id, timezone)
VALUES (sqlc.arg(user_id), sqlc.arg(timezone))
ON CONFLICT (user_id) DO UPDATE SET timezone = EXCLUDED.timezone, updated_at = now();
