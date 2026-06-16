-- name: CreateSubscription :one
-- Subscribe one of the caller's saved searches to a delivery channel. The SELECT
-- from saved_searches enforces ownership in the same statement: a saved_search_id
-- the caller does not own yields no row (sqlc :one returns ErrNoRows, mapped to
-- 404). A second subscription for the same (saved_search, channel) violates the
-- UNIQUE constraint (surfaced as a 409). Returns the created row.
INSERT INTO subscriptions (user_id, saved_search_id, channel, destination)
SELECT ss.user_id, ss.id, sqlc.arg(channel), sqlc.narg(destination)
FROM saved_searches ss
WHERE ss.id = sqlc.arg(saved_search_id) AND ss.user_id = sqlc.arg(user_id)
RETURNING *;

-- name: ListSubscriptions :many
-- The caller's subscriptions joined to each saved search's display name and query,
-- newest first — the "My subscriptions" view.
SELECT s.*, ss.name AS saved_search_name, ss.query AS saved_search_query
FROM subscriptions s
JOIN saved_searches ss ON ss.id = s.saved_search_id
WHERE s.user_id = $1
ORDER BY s.created_at DESC;

-- name: SetSubscriptionActive :one
-- Pause/resume a subscription, scoped to its owner. No matching owner-scoped row
-- returns no row (the handler maps that to 404).
UPDATE subscriptions
SET active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id)
RETURNING *;

-- name: DeleteSubscription :execrows
-- Unsubscribe, scoped to its owner. Returns the affected row count: 0 means it
-- does not exist or is not the caller's (the handler maps that to 404). The match
-- ledger cascades away with the subscription.
DELETE FROM subscriptions
WHERE id = $1 AND user_id = $2;

-- name: ListActiveSubscriptions :many
-- Every active subscription with the data the matching worker needs: the saved
-- search query to translate into a filter, plus identity/channel for fan-out. The
-- worker groups these by canonical(query) so each distinct filter hits the search
-- index once regardless of how many subscriptions share it.
SELECT s.id, s.user_id, s.channel, s.destination, s.start_at, ss.query
FROM subscriptions s
JOIN saved_searches ss ON ss.id = s.saved_search_id
WHERE s.active;

-- name: RecordSubscriptionMatch :execrows
-- Record that a job matched a subscription. The PK (subscription_id, job_id) makes
-- this idempotent — re-scanning an already-recorded match is a no-op — so the
-- worker can re-scan recent jobs freely without ever delivering twice. Returns the
-- affected row count (1 = newly recorded, 0 = already known).
INSERT INTO subscription_matches (subscription_id, job_id)
VALUES ($1, $2)
ON CONFLICT (subscription_id, job_id) DO NOTHING;

-- name: ClaimSubscriptionMatches :many
-- Lease a batch of pending, live matches for active subscriptions by stamping
-- claimed_at, oldest-claimable first, ordered by subscription so the worker can
-- group a subscription's matches into one digest. FOR UPDATE OF m locks only match
-- rows; SKIP LOCKED lets overlapping passes take disjoint rows so a digest is sent
-- at most once; the lease predicate reclaims rows whose sender died (stale
-- claimed_at), so no separate reaper is needed. The digest is sent OUTSIDE this
-- claim's transaction, so no network call is held inside a row lock.
WITH claimable AS (
    SELECT m.subscription_id, m.job_id
    FROM subscription_matches m
    JOIN subscriptions s ON s.id = m.subscription_id
    WHERE m.notified_at IS NULL
      AND m.failed_at IS NULL
      AND (m.claimed_at IS NULL
           OR m.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
      AND s.active
    -- Grouped by subscription so the delivery loop can build one digest each.
    -- The matched_at/job_id tiebreak makes a batch_size cut deterministic (oldest
    -- matches land in this batch); a subscription with more than batch_size pending
    -- matches splits across passes into multiple digests — an accepted rare case.
    ORDER BY m.subscription_id, m.matched_at, m.job_id
    FOR UPDATE OF m SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE subscription_matches m
SET claimed_at = now()
FROM claimable c
WHERE m.subscription_id = c.subscription_id AND m.job_id = c.job_id
RETURNING m.subscription_id, m.job_id;

-- name: GetSubscriptionForDelivery :one
-- The delivery context for one subscription: channel + destination, the saved
-- search name (for the digest heading), and the user's linked Telegram chat (NULL
-- when unlinked → the worker soft-skips telegram delivery rather than failing it).
SELECT s.id, s.user_id, s.channel, s.destination,
       ss.name AS saved_search_name,
       tl.chat_id AS telegram_chat_id
FROM subscriptions s
JOIN saved_searches ss ON ss.id = s.saved_search_id
LEFT JOIN telegram_links tl ON tl.user_id = s.user_id
WHERE s.id = $1;

-- name: GetJobsForDigest :many
-- The display fields for the jobs in a digest, freshest first.
SELECT id, title, company, public_slug, url, posted_at
FROM jobs
WHERE id = ANY(sqlc.arg(job_ids)::bigint[])
ORDER BY COALESCE(posted_at, created_at) DESC;

-- name: MarkMatchesNotified :execrows
-- Stamp notified_at on the jobs that were just delivered for a subscription, so
-- they leave the pending queue and are never sent again.
UPDATE subscription_matches
SET notified_at = now()
WHERE subscription_id = sqlc.arg(subscription_id)
  AND job_id = ANY(sqlc.arg(job_ids)::bigint[]);

-- name: RecordMatchDeliveryFailure :exec
-- Count a failed delivery for a subscription's claimed jobs: bump attempts, record
-- the error, and dead-letter (set failed_at) once attempts reach the max. claimed_at
-- is left in place — its expiry gates the retry to a later pass and doubles as the
-- crash reaper, mirroring enrichment_outbox.
UPDATE subscription_matches
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now()
                     ELSE NULL
                 END
WHERE subscription_id = sqlc.arg(subscription_id)
  AND job_id = ANY(sqlc.arg(job_ids)::bigint[]);

-- name: ReleaseMatchClaim :exec
-- Release the lease on a subscription's claimed jobs without counting an attempt,
-- so a soft-skipped delivery (e.g. Telegram not yet linked) is retried promptly on
-- a later pass instead of waiting out the lease.
UPDATE subscription_matches
SET claimed_at = NULL
WHERE subscription_id = sqlc.arg(subscription_id)
  AND job_id = ANY(sqlc.arg(job_ids)::bigint[]);
