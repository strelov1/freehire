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

-- name: RecordSubscriptionMatches :execrows
-- Record that a batch of (subscription, job) pairs matched, one round trip for
-- however many pairs one query's search hits produced across every subscription that
-- shares it — a popular query with many subscribers no longer costs one sequential
-- INSERT per (hit, subscription) pair. The PK (subscription_id, job_id) makes this
-- idempotent — re-scanning an already-recorded match is a no-op — so the worker can
-- re-scan recent jobs freely without ever delivering twice. Returns the affected row
-- count (newly recorded pairs; already-known pairs are silently skipped).
--
-- Two same-length unnest calls in the SELECT list, not a two-argument unnest(a, b): the
-- query analyzer cannot type the latter (see jobs.sql's MarkFuzzyDuplicatesForCompany,
-- same pattern for the same reason); Postgres runs same-length SELECT-list set-returning
-- functions in lockstep, pairing subscription_ids[i] with job_ids[i].
INSERT INTO subscription_matches (subscription_id, job_id)
SELECT unnest(sqlc.arg(subscription_ids)::bigint[]) AS subscription_id,
       unnest(sqlc.arg(job_ids)::bigint[]) AS job_id
ON CONFLICT (subscription_id, job_id) DO NOTHING;

-- name: ClaimSubscriptionMatches :many
-- Lease pending, live matches for active subscriptions by stamping claimed_at,
-- AT MOST per_subscription of them per subscription, so one busy subscription cannot
-- starve the rest. FOR UPDATE OF the match rows with SKIP LOCKED lets overlapping passes
-- take disjoint rows so a digest is sent at most once; the lease predicate reclaims rows
-- whose sender died (stale claimed_at), so no separate reaper is needed. The digest is
-- sent OUTSIDE this claim's transaction, so no network call is held inside a row lock.
--
-- The per-subscription cap is the whole point. This used to be one flat
-- `ORDER BY subscription_id, matched_at LIMIT batch_size` over every pending row, which
-- reads as "oldest first" but is really "lowest subscription id first": a subscription
-- whose filter matches most of the catalogue fills the batch every pass and every
-- higher id is never reached. Measured on prod 2026-09-04 — one subscription with an
-- EMPTY query held 248k pending matches, the queue was 1.14M deep, and subscriptions
-- above it had attempts=0 since the day they were created. Nothing was broken; they were
-- simply never in a batch.
--
-- A LATERAL per subscription, rather than a window function, because FOR UPDATE cannot
-- be used with window functions — and the row lock is what makes concurrent passes safe.
WITH claimable AS (
    SELECT m.subscription_id, m.job_id
    FROM subscriptions s
    CROSS JOIN LATERAL (
        SELECT mm.subscription_id, mm.job_id
        FROM subscription_matches mm
        WHERE mm.subscription_id = s.id
          AND mm.notified_at IS NULL
          AND mm.failed_at IS NULL
          AND (mm.claimed_at IS NULL
               OR mm.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
        -- Oldest first WITHIN the subscription: a digest that splits across passes sends
        -- the matches in the order they were found.
        ORDER BY mm.matched_at, mm.job_id
        LIMIT sqlc.arg(per_subscription)
        FOR UPDATE SKIP LOCKED
    ) m
    WHERE s.active
    -- A backstop, not the working bound: per_subscription x active subscriptions is what
    -- a pass normally claims. This only caps a pathological subscription count.
    LIMIT sqlc.arg(batch_size)
)
UPDATE subscription_matches m
SET claimed_at = now()
FROM claimable c
WHERE m.subscription_id = c.subscription_id AND m.job_id = c.job_id
RETURNING m.subscription_id, m.job_id;

-- name: GetSubscriptionForDelivery :one
-- The delivery context for one subscription: channel + destination, the saved
-- search name (for the digest heading), the user's account email (the email
-- channel's live recipient), the user's linked Telegram chat (NULL when unlinked
-- → the worker soft-skips telegram delivery rather than failing it), whether
-- the user has at least one registered push device (the push channel's live
-- deliverability check, same soft-skip role as the Telegram link), the user's
-- webhook destination (URL, NULL or disabled → the worker soft-skips webhook
-- delivery the same way), and the delivery-timing context
-- (live, not snapshotted, same as the channel checks above) — the account's
-- timezone and its saved-search digest frequency settings, read via
-- internal/application/deliverywindow before a digest is sent.
SELECT s.id, s.user_id, s.channel, s.destination, s.last_digest_sent_at,
       ss.name AS saved_search_name,
       u.email AS account_email,
       u.timezone AS timezone,
       tl.chat_id AS telegram_chat_id,
       EXISTS(SELECT 1 FROM user_push_tokens upt WHERE upt.user_id = s.user_id) AS has_push_device,
       wc.url AS webhook_url,
       COALESCE(wc.enabled, false) AS webhook_enabled,
       COALESCE(ns.digest_frequency, 'instant')::text AS digest_frequency,
       ns.digest_time AS digest_time,
       ns.quiet_hours_start AS quiet_hours_start,
       ns.quiet_hours_end AS quiet_hours_end
FROM subscriptions s
JOIN saved_searches ss ON ss.id = s.saved_search_id
JOIN users u ON u.id = s.user_id
LEFT JOIN telegram_links tl ON tl.user_id = s.user_id
LEFT JOIN webhook_configs wc ON wc.user_id = s.user_id
LEFT JOIN notification_settings ns ON ns.user_id = s.user_id
WHERE s.id = $1;

-- name: MarkDigestSent :exec
-- Stamp the subscription's last daily-digest send instant, so
-- internal/application/deliverywindow.DigestDue reads "already sent today" on any later pass
-- within the same local calendar day. Only called after a successful `daily`-mode
-- delivery — `instant`-mode subscriptions never touch this column.
UPDATE subscriptions
SET last_digest_sent_at = now()
WHERE id = $1;

-- name: GetJobsForDigest :many
-- The display fields for the jobs in a digest, freshest first. Salary fields are
-- projected out of the enrichment JSONB (absent keys → NULL) so a card can render
-- a compensation line only when one is known.
SELECT id, title, company, public_slug, url, posted_at,
       COALESCE((enrichment->>'salary_min')::int, 0)::int AS salary_min,
       COALESCE((enrichment->>'salary_max')::int, 0)::int AS salary_max,
       COALESCE(enrichment->>'salary_currency', '')::text AS salary_currency,
       COALESCE(enrichment->>'salary_period', '')::text   AS salary_period
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
