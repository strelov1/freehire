-- name: UpsertBookingLink :one
-- Turn on the caller's booking link for one of their applications. Ownership is the SELECT:
-- someone else's application inserts nothing and returns no row, which reads as not found.
-- A second call keeps the seed (so the link the candidate already shared still works) and
-- takes the zone the browser sent this time.
INSERT INTO booking_links (user_id, application_id, token_seed, token_hash, zone)
SELECT a.user_id, a.id, sqlc.arg(token_seed), sqlc.arg(token_hash), sqlc.arg(zone)
  FROM applications a
 WHERE a.id = sqlc.arg(application_id) AND a.user_id = sqlc.arg(user_id)
ON CONFLICT (application_id) DO UPDATE SET zone = EXCLUDED.zone
RETURNING id, user_id, application_id, token_seed, token_hash, zone;

-- name: GetBookingLinkForApplication :one
SELECT id, user_id, application_id, token_seed, token_hash, zone
  FROM booking_links
 WHERE user_id = sqlc.arg(user_id) AND application_id = sqlc.arg(application_id);

-- name: GetBookingLinkByHash :one
SELECT id, user_id, application_id, token_seed, token_hash, zone
  FROM booking_links
 WHERE token_hash = sqlc.arg(token_hash);

-- name: SetBookingLinkTokenHash :exec
-- Re-point a link at the token its seed now derives to, after the server key changed.
UPDATE booking_links SET token_hash = sqlc.arg(token_hash) WHERE id = sqlc.arg(id);

-- name: ListBookingLinkBusy :many
-- The owner's busy time overlapping a window, for the slot engine: their confirmed
-- booking-link meetings, plus every other confirmed meeting freehire already shows on their
-- applications. Interviews a booking link wrote are skipped (counted once, as bookings); an
-- interview with no end is held for 30 minutes.
SELECT b.starts_at, b.ends_at
  FROM booking_link_bookings b
 WHERE b.user_id = sqlc.arg(user_id)
   AND b.status = 'confirmed'
   AND b.starts_at < sqlc.arg(to_at)
   AND b.ends_at > sqlc.arg(from_at)
UNION ALL
SELECT i.starts_at, coalesce(i.ends_at, i.starts_at + interval '30 minutes') AS ends_at
  FROM application_interviews i
 WHERE i.user_id = sqlc.arg(user_id)
   AND i.status = 'confirmed'
   AND i.source <> 'booking_link'
   AND i.starts_at < sqlc.arg(to_at)
   AND coalesce(i.ends_at, i.starts_at + interval '30 minutes') > sqlc.arg(from_at)
ORDER BY starts_at;

-- name: InsertBookingLinkBooking :one
-- The EXCLUDE booking_link_bookings_no_overlap is what refuses a taken slot.
INSERT INTO booking_link_bookings (link_id, user_id, starts_at, ends_at, recruiter_name, recruiter_email)
VALUES (sqlc.arg(link_id), sqlc.arg(user_id), sqlc.arg(starts_at), sqlc.arg(ends_at),
        sqlc.arg(recruiter_name), sqlc.arg(recruiter_email))
RETURNING id;
