-- name: GetOrInitFeedSeen :one
-- The feed watermark for a (user, source). The first read stores `init_at` (a first visit has
-- no new cards); later reads return the stored value. The no-op DO UPDATE makes RETURNING
-- work on a conflict too, and stays race-safe where INSERT-then-SELECT would not be.
INSERT INTO feed_state (user_id, source, seen_at)
VALUES (sqlc.arg(user_id), sqlc.arg(source), sqlc.arg(init_at)::timestamptz)
ON CONFLICT (user_id, source) DO UPDATE SET seen_at = feed_state.seen_at
RETURNING seen_at;

-- name: AdvanceFeedSeen :exec
-- Move the watermark forward, never back: an older snapshot posted late changes nothing.
UPDATE feed_state SET seen_at = GREATEST(seen_at, sqlc.arg(at)::timestamptz)
 WHERE user_id = sqlc.arg(user_id) AND source = sqlc.arg(source);

-- name: SearchDrainHighWater :one
-- The oldest open job (created in the last 24h) still waiting in search_outbox, or NULL when
-- nothing is pending. A feed snapshot stays below it, so a late-indexed job is new on the
-- next visit instead of never. Derived from jobs, not from the outbox alone, so a backfill
-- requeue of old jobs cannot pin it.
-- shortcut: a job stuck in the outbox longer than 24h can never be new; widen the window if
-- the outbox-age alert shows longer stalls.
SELECT min(j.created_at)::timestamptz AS high_water FROM jobs j
 WHERE j.closed_at IS NULL AND j.created_at > now() - interval '24 hours'
   AND EXISTS (SELECT 1 FROM search_outbox o WHERE o.job_id = j.id AND o.failed_at IS NULL);
