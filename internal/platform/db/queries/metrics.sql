-- Aggregates behind cmd/queue-metrics, the worker that publishes pipeline depth to
-- Prometheus. Every query here is read-only and takes no locks by design: it runs once a
-- minute alongside ingest, search-drain, and reindex, and must never be the reason one of
-- them waits.

-- name: SearchOutboxMetrics :one
-- Live depth, dead-letter count, and the age of the oldest live entry, in one pass.
--
-- Postgres scans the whole table for the count either way, so computing min(created_at)
-- from the same scan is free — three round trips for the same three numbers would not be.
--
-- The age COALESCEs to 0 for an empty queue rather than staying NULL: a drained queue is
-- a real measurement that must publish an explicit zero, because an absent series is how
-- the consuming alert rules recognize a dead exporter.
SELECT
    count(*) FILTER (WHERE failed_at IS NULL)     AS depth,
    count(*) FILTER (WHERE failed_at IS NOT NULL) AS dead_letters,
    COALESCE(
        EXTRACT(EPOCH FROM now() - min(created_at) FILTER (WHERE failed_at IS NULL)),
        0
    )::float8                                     AS oldest_age_seconds
FROM search_outbox;

-- name: EnrichmentOutboxMetrics :one
-- Same shape and same reasoning as SearchOutboxMetrics. The three outbox tables share a
-- column layout, but the queries stay separate and literal: sqlc generates from static
-- SQL, so a parameterized table name would cost both the codegen and the schema's
-- compile-time guarantee to save two dozen lines.
SELECT
    count(*) FILTER (WHERE failed_at IS NULL)     AS depth,
    count(*) FILTER (WHERE failed_at IS NOT NULL) AS dead_letters,
    COALESCE(
        EXTRACT(EPOCH FROM now() - min(created_at) FILTER (WHERE failed_at IS NULL)),
        0
    )::float8                                     AS oldest_age_seconds
FROM enrichment_outbox;

-- name: SemanticOutboxMetrics :one
-- Same shape and same reasoning as SearchOutboxMetrics.
SELECT
    count(*) FILTER (WHERE failed_at IS NULL)     AS depth,
    count(*) FILTER (WHERE failed_at IS NOT NULL) AS dead_letters,
    COALESCE(
        EXTRACT(EPOCH FROM now() - min(created_at) FILTER (WHERE failed_at IS NULL)),
        0
    )::float8                                     AS oldest_age_seconds
FROM semantic_outbox;

-- name: MailClassificationOutboxMetrics :one
-- Same shape and same reasoning as SearchOutboxMetrics.
--
-- This queue was the one of the four that nothing measured, and it is the one that failed
-- silently for five weeks: cmd/classify-mail dead-lettered every message, then logged
-- "done failed=0 dead-lettered=0" on each subsequent run — accurate, because a dead entry is
-- never claimed again, and indistinguishable from an empty queue. Dead letters here read as
-- mail nobody will ever link to an application.
SELECT
    count(*) FILTER (WHERE failed_at IS NULL)     AS depth,
    count(*) FILTER (WHERE failed_at IS NOT NULL) AS dead_letters,
    COALESCE(
        EXTRACT(EPOCH FROM now() - min(created_at) FILTER (WHERE failed_at IS NULL)),
        0
    )::float8                                     AS oldest_age_seconds
FROM email_classification_outbox;

-- name: AppleRevocationJobMetrics :one
-- Same shape as SearchOutboxMetrics, over a queue that records its state in a status
-- column instead of a failed_at stamp, so the two FILTERs spell the same split in its
-- own vocabulary: anything a run can still claim is depth, and `failed` is the dead
-- letter — no run claims it again, and an abandoned `unlink` leaves its user_identities
-- row in revocation_pending, where nothing can ever unlink that identity.
--
-- `processing` counts as live: the claim takes back a row stuck in it for ten minutes, so
-- it is work still owed rather than work given up on.
SELECT
    count(*) FILTER (WHERE status IN ('pending', 'retry', 'processing')) AS depth,
    count(*) FILTER (WHERE status = 'failed')                            AS dead_letters,
    COALESCE(
        EXTRACT(EPOCH FROM now() - min(created_at)
            FILTER (WHERE status IN ('pending', 'retry', 'processing'))),
        0
    )::float8                                                            AS oldest_age_seconds
FROM apple_revocation_jobs;

-- name: SearchDeleteOutboxMetrics :one
-- Same shape and same reasoning as SearchOutboxMetrics, over the removal side of the
-- facet index.
--
-- Its dead letters are the sharper half of the pair. An entry this queue gives up on
-- leaves a CLOSED posting in the live index — searchable, clickable, and 404ing on
-- arrival — until the next full rebuild sweeps it out, and search_delete_outbox's
-- UNIQUE (job_id) plus the ON CONFLICT DO NOTHING every enqueue carries means that
-- posting can never be queued for removal again in the meantime.
SELECT
    count(*) FILTER (WHERE failed_at IS NULL)     AS depth,
    count(*) FILTER (WHERE failed_at IS NOT NULL) AS dead_letters,
    COALESCE(
        EXTRACT(EPOCH FROM now() - min(created_at) FILTER (WHERE failed_at IS NULL)),
        0
    )::float8                                     AS oldest_age_seconds
FROM search_delete_outbox;

-- name: ApplyFormOutboxMetrics :one
-- Same shape and same reasoning as SearchOutboxMetrics. The depth here is the one that
-- carries information rather than the dead letters: cmd/capture-apply-form takes
-- APPLY_FORM_MAX_PER_RUN entries a run against a backlog that started at ~185k, so
-- whether the drain is gaining on it is a question only this gauge answers.
SELECT
    count(*) FILTER (WHERE failed_at IS NULL)     AS depth,
    count(*) FILTER (WHERE failed_at IS NOT NULL) AS dead_letters,
    COALESCE(
        EXTRACT(EPOCH FROM now() - min(created_at) FILTER (WHERE failed_at IS NULL)),
        0
    )::float8                                     AS oldest_age_seconds
FROM apply_form_outbox;

-- name: AdzunaDescriptionOutboxMetrics :one
-- Same shape and same reasoning as SearchOutboxMetrics. Same bounded-drain question as
-- apply_form_outbox: ADZUNA_DESCRIPTION_MAX_PER_RUN defaults to 500 and is deliberately
-- conservative, so the depth is what says whether that figure is still the right one.
SELECT
    count(*) FILTER (WHERE failed_at IS NULL)     AS depth,
    count(*) FILTER (WHERE failed_at IS NOT NULL) AS dead_letters,
    COALESCE(
        EXTRACT(EPOCH FROM now() - min(created_at) FILTER (WHERE failed_at IS NULL)),
        0
    )::float8                                     AS oldest_age_seconds
FROM adzuna_description_outbox;

-- name: AutoApplyQueueMetrics :one
-- Same idea as SearchOutboxMetrics, but this queue has THREE terminal-ish states rather
-- than two, so it cannot be a copy of it.
--
-- Depth is ClaimAutoApplyBatch's own predicate minus the lease: a submission attempt is
-- only ever claimed once the candidate has approved a tailored CV for it, so an entry
-- still awaiting that review is deliberately NOT depth — it is waiting on a person, and
-- counting it would make ordinary candidate behaviour look like a stalled worker.
--
-- Blocked is the third state and the reason this needs its own gauge. `Park is not a
-- retry` (internal/application/autoapply/AGENTS.md): an attempt the sidecar could not
-- fully resolve is excluded from the claim by blocked_at without touching attempts, so
-- it is neither work still owed nor work retried to exhaustion. Folding it into either
-- of the other two counts would be a wrong reading, and leaving it out entirely is how a
-- population that no run will ever pick up again becomes invisible.
--
-- A dead letter here has TWO markers, not one. preview_failed_at (migration 0140,
-- written by RecordAutoApplyPreviewFailure) retires the preview pass the same way
-- failed_at retires the submit pass, and a row carrying only the first is claimed by
-- neither: ClaimAutoApplyPreviewBatch excludes it directly, and ClaimAutoApplyBatch
-- excludes it because review_decision never reaches 'approved' without a preview. Reading
-- only failed_at is therefore the exact omission the paragraph above forbids, and
-- auto_apply_review_info.go already reads both for the same reason.
--
-- A DECLINE is not parked work. DeclineAutoApplyReview reuses MarkAutoApplyBlocked's
-- vocabulary and stamps blocked_at, nothing ever deletes the row, so counting it would
-- make the gauge grow with ordinary candidate behaviour until the sidecar population it
-- exists to publish is a rounding error inside it. Both other readers of this state guard
-- the same way and say so — autoapply/status.go and auto_apply_enqueue.go.
--
-- The dead-letter markers win over blocked_at where a row carries both, so the three
-- counts stay mutually exclusive and a stacked graph reads correctly.
SELECT
    count(*) FILTER (
        WHERE failed_at IS NULL
          AND blocked_at IS NULL
          AND tailored_cv_id IS NOT NULL
          AND review_decision = 'approved'
    )                                                                AS depth,
    count(*) FILTER (
        WHERE failed_at IS NOT NULL OR preview_failed_at IS NOT NULL
    )                                                                AS dead_letters,
    count(*) FILTER (
        WHERE failed_at IS NULL
          AND preview_failed_at IS NULL
          AND blocked_at IS NOT NULL
          AND review_decision IS DISTINCT FROM 'declined'
    )                                                                AS blocked,
    COALESCE(
        EXTRACT(EPOCH FROM now() - min(created_at) FILTER (
            WHERE failed_at IS NULL
              AND blocked_at IS NULL
              AND tailored_cv_id IS NOT NULL
              AND review_decision = 'approved'
        )),
        0
    )::float8                                                        AS oldest_age_seconds
FROM auto_apply_queue;

-- name: PushTicketOutboxMetrics :one
-- Depth and age only: this queue carries neither attempts nor failed_at, because
-- DeletePushTickets removes a ticket on every terminal receipt outcome and an
-- unprocessed batch (Expo unreachable) simply stays queued for the next run. There is no
-- give-up state to measure, so the caller publishes no dead-letter sample for it rather
-- than a zero that would claim a measurement nobody took.
--
-- That makes the AGE the whole signal here. Every row is live by construction, so a
-- depth that merely grows says the fleet is busy, while an oldest entry older than a few
-- scheduled runs says cmd/push-receipts is not draining — the one failure this queue can
-- have and the one it would otherwise report as a perfectly healthy backlog.
SELECT
    count(*)                                                    AS depth,
    COALESCE(EXTRACT(EPOCH FROM now() - min(created_at)), 0)::float8 AS oldest_age_seconds
FROM push_ticket_outbox;

-- name: NewestOpenJobCreatedAt :one
-- When the catalogue last gained a posting, for the "ingest has stopped producing" signal.
--
-- Deliberately ORDER BY ... LIMIT 1 over OPEN jobs rather than max(created_at) over all of
-- them: jobs_open_created_idx is (created_at DESC, id DESC) WHERE closed_at IS NULL, so
-- this is a single index probe, while an unfiltered max() has no index to use and would
-- seq-scan millions of rows every minute. Open is also the honest population — a freshly
-- ingested job is open, and a closure is not catalogue growth.
--
-- Returns no row when the catalogue is empty, which the caller must distinguish from a
-- zero timestamp: zero reads as 1970, i.e. an infinitely stale catalogue, whereas an
-- empty catalogue is a fresh-install state and not an incident.
--
-- AND NOT is_private for the same reason the public twin of this question carries it
-- (jobs.sql's LatestOpenJobAddedAt): a jd-tailor-intake row is a user pasting a job
-- description in, not the crawler producing one, so counting it would let one paste hide
-- an ingest that has stopped — the exact incident this gauge exists to raise.
SELECT created_at
FROM jobs
WHERE closed_at IS NULL AND NOT is_private
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: NotifyBacklogMetrics :many
-- The subscription-digest backlog, split by whether the subscription can be delivered at
-- all: how many active subscriptions have something undelivered, and how old the oldest
-- undelivered match is, per value of `deliverable`.
--
-- The age is the signal that matters. A pass runs every five minutes, so in steady state
-- the oldest pending match of a deliverable instant subscription is minutes old (an
-- interval digest's, up to its interval; a daily digest's, up to a day). An age that climbs without bound means some subscription is never
-- being served — which is not visible in the worker's own log, because a starved
-- subscription produces no failure: `notify` reported `delivered=1 failed=0` for weeks while
-- 1.14M matches sat undelivered and one subscription's had never been claimed at all
-- (2026-09-04, see docs/agents/notifications.md).
--
-- Stale matches (expired_at, migration 0193) are not pending, so the age is bounded by the
-- notify worker's freshness window (3 days). A subscription that cannot be delivered at all
-- — push with no device, an unlinked Telegram chat, a disabled webhook — holds matches up to
-- that age and then lets them go, so before the split the one age sat at the window for
-- good and said nothing. `deliverable = false` is that population; the alertable age is
-- the `true` one.
--
-- The CASE is the SQL twin of recipient() in internal/engage/notify/notify.go, over the
-- same live columns GetSubscriptionForDelivery reads (queries/subscription_delivery.sql). Change
-- one and change the other: a metric that disagrees with the sender about who is reachable
-- would page on a subscription the sender correctly skips, or hide one it fails.
--
-- A deliverability state with no pending subscription yields no row; cmd/queue-metrics
-- publishes it as an explicit zero, because an absent series is how the consuming alert
-- rules recognize a dead exporter. Aggregating matches per subscription first keeps the
-- per-match work on subscription_matches_pending_matched_idx; measured on prod 2026-10-22
-- (~40k pending matches, 64 pending subscriptions): 34ms.
WITH pending AS (
    SELECT m.subscription_id, min(m.matched_at) AS oldest_matched_at
    FROM subscription_matches m
    JOIN subscriptions ps ON ps.id = m.subscription_id
    JOIN saved_searches pss ON pss.id = ps.saved_search_id
    WHERE m.notified_at IS NULL AND m.failed_at IS NULL AND m.expired_at IS NULL
      AND m.query_revision = pss.query_revision
    GROUP BY m.subscription_id
)
SELECT
    (CASE s.channel
        WHEN 'telegram' THEN u.banned_at IS NULL AND tl.chat_id IS NOT NULL
        WHEN 'email'    THEN mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at) <> ''
        WHEN 'push'     THEN u.banned_at IS NULL AND EXISTS (SELECT 1 FROM user_push_tokens upt WHERE upt.user_id = s.user_id)
        WHEN 'webhook'  THEN u.banned_at IS NULL AND COALESCE(wc.enabled, false) AND COALESCE(wc.url, '') <> ''
        ELSE COALESCE(s.destination, '') <> ''
    END)::boolean AS deliverable,
    count(*)::bigint AS pending_subscriptions,
    EXTRACT(EPOCH FROM now() - min(p.oldest_matched_at))::float8 AS oldest_age_seconds
FROM pending p
JOIN subscriptions s ON s.id = p.subscription_id
JOIN users u ON u.id = s.user_id
LEFT JOIN telegram_links tl ON tl.user_id = s.user_id
LEFT JOIN webhook_configs wc ON wc.user_id = s.user_id
WHERE s.active
GROUP BY 1
ORDER BY 1 DESC;

-- name: NotifyDigestVolumeMetrics :one
-- How many subscription digests reached people in the last 24 hours, and how hard the
-- busiest recipients were hit. The backlog above says whether digests go OUT; this says
-- whether too MANY do, which nothing measured until a user complained (2026-11-05): a
-- server-side rewrite of 578 profile alerts widened them, and one person got 43 digests in
-- a day. The 24 in users_over_daily_cap is the per-subscription ceiling of the hourly
-- default; a user above it is getting more than one subscription's worth.
--
-- profile_alerts_rewritten_1h counts profile-derived saved searches UPDATED in the last
-- hour that existed before it — a creation is not a rewrite, so onboarding does not
-- read as one. updated_at also moves on a rename, which this cannot tell apart; a
-- profile alert is named "My profile" and almost never renamed, so the count is the
-- query rewrites in practice. A user edits one at a time; hundreds in an hour is a mass
-- rewrite, the step that started the 2026-11-04 flood.
--
-- Per user, not per subscription or channel: the complaint is about one inbox. Measured on
-- prod 2026-11-05 (16k digests, 1.5k recipients): 25ms warm, on
-- user_notifications_user_id_created_at_idx.
WITH per_user AS (
    SELECT user_id, count(*) AS n
    FROM user_notifications
    WHERE kind = 'subscription_digest' AND created_at > now() - interval '24 hours'
    GROUP BY user_id
)
SELECT
    COALESCE(sum(n), 0)::bigint AS digests,
    count(*)::bigint AS recipients,
    COALESCE(max(n), 0)::bigint AS max_per_user,
    (count(*) FILTER (WHERE n > 24))::bigint AS users_over_daily_cap,
    (SELECT count(*) FROM saved_searches
     WHERE derived_from_profile AND updated_at > now() - interval '1 hour'
       AND created_at <= now() - interval '1 hour')::bigint AS profile_alerts_rewritten_1h
FROM per_user;
