-- The shared one-optional-letter-a-day claim (internal/engage/maildaily). Every sender of
-- optional mail takes it before calling SES, so an account gets at most one such letter
-- per UTC day whichever worker reaches it first. See migration 0248.

-- name: ClaimMailDay :one
-- Takes today's slot for one account. No row back (pgx.ErrNoRows) means somebody else
-- already holds it and the caller must not send. The day is returned so a release after
-- a failure that straddles midnight removes the row that was written, not today's.
INSERT INTO mail_day_claims (user_id, day, source)
VALUES (sqlc.arg(user_id), mail_today(), sqlc.arg(source)::text)
ON CONFLICT (user_id, day) DO NOTHING
RETURNING day;

-- name: ReleaseMailDay :exec
-- Gives the slot back after a DEFINITE failure to send. Scoped to the source that took
-- it, so a sender can never free a slot another one is relying on.
DELETE FROM mail_day_claims
WHERE user_id = sqlc.arg(user_id)
  AND day = sqlc.arg(day)
  AND source = sqlc.arg(source)::text;

-- name: PruneMailDayClaims :execrows
-- Claims only matter for the day they were taken; a week of them is kept for the
-- one-letter-a-day report and the rest is dropped.
DELETE FROM mail_day_claims
WHERE day < mail_today() - 7;
