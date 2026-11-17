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

-- name: GetSubscriptionTarget :one
-- What an owner-scoped subscription delivers: its channel and its saved search's
-- query. Resuming a paused subscription re-applies the gates a new one passes, and
-- these are what they read. No matching owner-scoped row returns no row (404).
SELECT s.channel, ss.query
FROM subscriptions s
JOIN saved_searches ss ON ss.id = s.saved_search_id
WHERE s.id = sqlc.arg(id) AND s.user_id = sqlc.arg(user_id);

-- name: DeactivateSubscriptions :execrows
-- Pause the given subscriptions. The matching worker calls it for a saved search
-- whose query the search can no longer execute: such a subscription fails every
-- pass and can never deliver, and the owner can fix the search and resume it.
-- Only while the saved search still holds the query the worker judged: an owner who
-- fixed it after the worker read it must not find the fixed alert paused.
UPDATE subscriptions s SET active = false
FROM saved_searches ss
WHERE ss.id = s.saved_search_id
  AND s.id = ANY(sqlc.arg(ids)::bigint[])
  AND s.active
  AND ss.query = sqlc.arg(query);

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
--
-- The alerts switch is applied HERE rather than at send time, so a delivery path
-- added later inherits the gate instead of having to remember it.
--
-- It gates the EMAIL channel only. Turning off "job alerts" means turning off the
-- mail: a Telegram or webhook destination is something the account connected
-- itself and turns off where it connected it, and silencing those from a link in
-- an email would be acting well beyond what the link said it would do.
--
-- LEFT JOIN with COALESCE(..., true): a missing notification_settings row means the
-- account never opened the settings page, which is not the same as opting out. Same
-- reading as broadcast.sql and onboarding.sql, and the opposite of nudges.sql's
-- inner join — see migration 0153 for why one column could not answer both.
--
-- It also carries the delivery-timing context (the same COALESCE defaults as
-- GetSubscriptionForDelivery and ClaimSubscriptionMatches) and the two stamps the
-- worker uses to skip a search whose result could not be sent this pass — see
-- notify.searchDue.
SELECT s.id, s.user_id, s.channel, s.destination, s.start_at, ss.query, ss.query_revision,
       s.last_digest_sent_at, s.last_searched_at,
       u.timezone AS timezone,
       -- EFFECTIVE frequency, not the stored one: faster than daily is a paid benefit (a live
       -- pro_until or ultra_until, whichever source granted it), so a Free account reads as
       -- 'daily' at its stored time whatever it kept or had before an entitlement expired.
       -- A webhook is a program's endpoint, not a person's digest, and keeps the stored value.
       -- The same expression gates ClaimSubscriptionMatches; the three must agree.
       (CASE WHEN s.channel = 'webhook'
                  OR COALESCE(u.pro_until > now(), false) OR COALESCE(u.ultra_until > now(), false)
             THEN COALESCE(ns.digest_frequency, 'daily') ELSE 'daily' END)::text AS digest_frequency,
       COALESCE(ns.digest_interval_hours, 1)::int AS digest_interval_hours,
       COALESCE(ns.digest_time, '09:00'::time) AS digest_time,
       ns.quiet_hours_start AS quiet_hours_start,
       ns.quiet_hours_end AS quiet_hours_end
FROM subscriptions s
JOIN saved_searches ss ON ss.id = s.saved_search_id
JOIN users u ON u.id = s.user_id
LEFT JOIN notification_settings ns ON ns.user_id = s.user_id
WHERE s.active
  AND (s.channel <> 'email' OR COALESCE(ns.alerts_email_enabled, true));

-- name: MarkSubscriptionsSearched :exec
-- Stamp the subscriptions whose saved search the matching stage just ran and
-- recorded. searched_at is the instant the pass STARTED, not now(): the next
-- search pages back to it, so a posting indexed while this pass ran is covered.
UPDATE subscriptions SET last_searched_at = sqlc.arg(searched_at)
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

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
--
-- Subscriptions may be deleted after ListActiveSubscriptions but before this write.
-- Join their current rows so one unsubscribe cannot fail the entire query group's
-- batch. Lock those parents through the insert: an existence check alone still races
-- with a DELETE that commits after this statement takes its snapshot.
--
-- Each pair carries the query revision its search ran under (a third aligned array), and a
-- pair is recorded only while the saved search is still at that revision: an edit that
-- commits between the pass reading the query and recording its hits must not let the old
-- query's results in after the edit cleared the pending set. The saved search row is locked
-- through the insert for the same reason as the subscription. On conflict, a still-pending
-- row of an OLDER revision is reset to the new one, or a job the new query also matches
-- would stay blocked behind the leftover row claiming ignores.
INSERT INTO subscription_matches (subscription_id, job_id, query_revision)
SELECT s.id, pairs.job_id, pairs.query_revision
FROM (
    SELECT unnest(sqlc.arg(subscription_ids)::bigint[]) AS subscription_id,
           unnest(sqlc.arg(job_ids)::bigint[]) AS job_id,
           unnest(sqlc.arg(query_revisions)::int[]) AS query_revision
) pairs
JOIN subscriptions s ON s.id = pairs.subscription_id
JOIN saved_searches ss ON ss.id = s.saved_search_id AND ss.query_revision = pairs.query_revision
FOR KEY SHARE OF s
FOR SHARE OF ss
ON CONFLICT (subscription_id, job_id) DO UPDATE
SET query_revision = EXCLUDED.query_revision,
    matched_at     = now(),
    claimed_at     = NULL,
    attempts       = 0,
    last_error     = '',
    failed_at      = NULL,
    expired_at     = NULL
WHERE subscription_matches.query_revision < EXCLUDED.query_revision
  AND subscription_matches.notified_at IS NULL;

-- name: ListUserEmailSubscriptions :many
-- The account's email digest subscriptions, named, for the public preference page.
-- Email only: the page is reached from an email and may only govern email, so
-- listing a Telegram subscription there would offer a control the page must not
-- have.
SELECT s.id, s.active, ss.name
FROM subscriptions s
JOIN saved_searches ss ON ss.id = s.saved_search_id
WHERE s.user_id = $1 AND s.channel = 'email'
ORDER BY ss.name;

-- name: DeactivateEmailSubscription :execrows
-- Turn ONE email digest off, scoped to its owner. Deactivate only — it cannot
-- create a subscription and cannot turn one back on, so a leaked link can silence
-- somebody but never sign them up for anything. Returns the affected row count; 0
-- means it was already off, or is not this account's, and the caller treats both
-- the same rather than revealing which.
UPDATE subscriptions
SET active = false
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND channel = 'email' AND active;

-- name: GetAlertChannelState :one
-- Which delivery channels can actually reach this account right now — the same live
-- checks the delivery worker makes before sending (GetSubscriptionForDelivery's
-- telegram_chat_id, has_push_device and webhook_url/enabled, and
-- ListActiveSubscriptions' alerts_email_enabled gate on the email channel). The signed-in
-- MCP server reads it so an agent can only subscribe on a channel that will deliver,
-- instead of creating a subscription whose matches are soft-skipped forever.
SELECT COALESCE(ns.alerts_email_enabled, true)::boolean AS email_alerts_enabled,
       (u.banned_at IS NULL AND EXISTS(SELECT 1 FROM telegram_links tl WHERE tl.user_id = u.id))::boolean AS telegram_linked,
       (u.banned_at IS NULL AND EXISTS(SELECT 1 FROM user_push_tokens upt WHERE upt.user_id = u.id))::boolean AS has_push_device,
       COALESCE(wc.enabled, false)::boolean AS webhook_enabled
FROM users u
LEFT JOIN notification_settings ns ON ns.user_id = u.id
LEFT JOIN webhook_configs wc ON wc.user_id = u.id AND u.banned_at IS NULL
WHERE u.id = $1;

-- name: ListSubscriptionDeliveryStats :many
-- Per subscription of one account: how many matches still wait to be delivered and when
-- one was last delivered. "Pending" is the claim's own predicate (not notified, failed or
-- expired), so the figure is what the next digest will draw from. A subscription with no
-- match yet has no row; the caller reads that as zero and never.
SELECT m.subscription_id,
       count(*) FILTER (WHERE m.notified_at IS NULL AND m.failed_at IS NULL AND m.expired_at IS NULL
                        AND m.query_revision = ss.query_revision) AS pending,
       max(m.notified_at)::timestamptz AS last_delivered_at
FROM subscription_matches m
JOIN subscriptions s ON s.id = m.subscription_id
JOIN saved_searches ss ON ss.id = s.saved_search_id
WHERE s.user_id = $1
GROUP BY m.subscription_id;

-- name: ListSubscriptionUpdates :many
-- The postings the notify worker matched to one account's subscriptions since a moment,
-- newest match first — the ledger the digests are built from, read rather than re-run,
-- so "what is new" answers exactly what the alerts were (or will be) about. Scoped to the
-- owner in SQL; saved_search_id narrows to one alert (every channel it is delivered on). A posting that has closed
-- since is left out: it is no longer something to apply to. So is a private posting, which
-- the matcher never records (it reads the public index) and which must not reach anyone else. Delivery state is returned
-- so the reader can tell an already-mailed match from one still queued.
SELECT m.subscription_id, ss.id AS saved_search_id, ss.name AS saved_search_name, m.matched_at, m.notified_at,
       j.title, j.company, j.location, j.public_slug, j.posted_at
FROM subscription_matches m
JOIN subscriptions s ON s.id = m.subscription_id
JOIN saved_searches ss ON ss.id = s.saved_search_id
JOIN jobs j ON j.id = m.job_id
WHERE s.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(saved_search_id)::bigint IS NULL OR ss.id = sqlc.narg(saved_search_id)::bigint)
  AND m.matched_at >= sqlc.arg(since)::timestamptz
  AND j.closed_at IS NULL
  AND NOT j.is_private
ORDER BY m.matched_at DESC, m.job_id DESC
LIMIT sqlc.arg(row_limit);
