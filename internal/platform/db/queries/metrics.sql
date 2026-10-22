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

-- name: ProviderIngestHealth :many
-- Per-provider ingest health: when the provider's most recent board crawl succeeded, and
-- how its boards split across four mutually exclusive states. It is also the ONLY source of
-- the fleet-wide freehire_boards_total, which cmd/queue-metrics sums from these rows, so the
-- per-provider and fleet-wide families cannot disagree about what a board is.
--
-- The population is the CATALOGUE — every board in `boards` that ingest still crawls
-- (status pending or active) — with board_health LEFT JOINed onto it. It used to be
-- board_health alone, and that was wrong in both directions, each time silently:
--
--   * A board the catalogue has never crawled has no board_health row, so a provider whose
--     boards are all new was not exported at all. Measured 2026-10-19: after a re-key, all
--     3,324 gupy boards were `pending` and uncrawled, and gupy had NO series in either
--     provider family — on the day its API had already been dead for 19 days with 73k open
--     postings going stale. Vanishing is how an outage becomes invisible: no alert can be
--     late about a series that is not there. 23,806 live boards across 26 providers had no
--     health row that day (careerplug alone 8,071).
--   * A RETIRED board keeps its board_health row unless the retiring path deletes it, so a
--     retired provider went on being exported. All 5,833 apploi boards were retired on
--     2026-09-15 (cmd/close-apploi-misattributed) and apploi still published 5,833 boards
--     and a last success 40 days old, which a "large provider has gone quiet" rule would
--     page on forever. 6,794 health rows matched no live board that day.
--
-- The join key is board_health's own identity, (provider, lower(board), region) — see
-- migration 0171 — which is also how the catalogue's boards_identity_key folds a live
-- board, so a live board meets at most one health row.
--
-- The four states:
--   uncrawled — in the catalogue, no health row: no crawl of it has ever been recorded.
--   cooled    — in cooldown, so NOT currently being crawled. Takes precedence over
--               failing: a cooled board always has failures behind it (pipeline's
--               CooldownFor), and counting it twice would make the states exceed the fleet.
--   failing   — out of cooldown with consecutive failures.
--   healthy   — out of cooldown, last crawl succeeded.
--
-- last_success_at is max() over the provider's LIVE boards only, so a retired board's old
-- success cannot make a provider look fresher than its current boards are. It is NULL for a
-- provider no live board of which has ever succeeded, and the caller publishes that as an
-- ABSENT sample, never a zero (a Unix zero reads as overdue since 1970).
--
-- first_listed_at is what an alert falls back to in exactly that case: the moment the
-- provider's OLDEST live board entered the catalogue. "Listed since X and never once
-- succeeded" is a lower bound on how long the provider has been producing nothing, so
-- `time() - (last_success or first_listed)` is an honest silence for every provider. The
-- OLDEST board, never the newest: with the newest, adding one board to a provider that is
-- failing would restart its clock and hide the outage for another day. It is
-- boards.created_at rather than board_health.first_seen_at because an uncrawled board has
-- no health row to carry one. It is never NULL (created_at is NOT NULL and every row here
-- has at least one live board). One limit, accepted: a re-key retires the old boards and
-- the new ones start a fresh clock, so silence that began before the re-key is not
-- counted — the re-key is a deliberate act by someone already looking at the provider.
--
-- Cost, measured on prod 2026-10-19 (213k live boards, 196k health rows): 228ms warm, a
-- parallel hash join of the two tables, against 37ms for the board_health-only form it
-- replaces. It also replaces BoardHealthMetrics, the fleet-wide twin it now feeds, so the
-- net cost is one query of this size a minute. Neither table is jobs, whose 8M-row scan is
-- what this file's header exists to forbid.
SELECT
    b.provider,
    max(h.last_success_at)::timestamptz AS last_success_at,
    min(b.created_at)::timestamptz      AS first_listed_at,
    count(*) FILTER (
        WHERE h.provider IS NULL
    ) AS uncrawled,
    count(*) FILTER (
        WHERE h.provider IS NOT NULL
          AND h.cooldown_until > now()
    ) AS cooled,
    count(*) FILTER (
        WHERE h.provider IS NOT NULL
          AND (h.cooldown_until IS NULL OR h.cooldown_until <= now())
          AND h.consecutive_failures > 0
    ) AS failing,
    count(*) FILTER (
        WHERE h.provider IS NOT NULL
          AND (h.cooldown_until IS NULL OR h.cooldown_until <= now())
          AND h.consecutive_failures = 0
    ) AS healthy
FROM boards b
LEFT JOIN board_health h
       ON h.provider = b.provider
      AND lower(h.board) = lower(b.board)
      AND h.region = b.region
WHERE b.status IN ('pending', 'active')
GROUP BY b.provider
ORDER BY b.provider;

-- name: NotifyBacklogMetrics :many
-- The subscription-digest backlog, split by whether the subscription can be delivered at
-- all: how many active subscriptions have something undelivered, and how old the oldest
-- undelivered match is, per value of `deliverable`.
--
-- The age is the signal that matters. A pass runs every five minutes, so in steady state
-- the oldest pending match of a deliverable instant subscription is minutes old (a daily
-- digest's, up to a day). An age that climbs without bound means some subscription is never
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
-- same live columns GetSubscriptionForDelivery reads (queries/subscriptions.sql). Change
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
    WHERE m.notified_at IS NULL AND m.failed_at IS NULL AND m.expired_at IS NULL
    GROUP BY m.subscription_id
)
SELECT
    (CASE s.channel
        WHEN 'telegram' THEN tl.chat_id IS NOT NULL
        WHEN 'email'    THEN u.email <> ''
        WHEN 'push'     THEN EXISTS (SELECT 1 FROM user_push_tokens upt WHERE upt.user_id = s.user_id)
        WHEN 'webhook'  THEN COALESCE(wc.enabled, false) AND COALESCE(wc.url, '') <> ''
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

-- name: IngestDuplicationMetrics :many
-- How many rows each source has just written, and how many DISTINCT postings those rows
-- stand for. The gap between the two is the signal: one posting stored under many boards.
--
-- It exists because nothing could see the apploi failure of 2026-09-02. api.apploi.com
-- stopped honouring its `employer` parameter, so all 5,833 of that provider's boards began
-- fetching the same global catalogue, and 1,565,701 rows accumulated standing for 3,024
-- real jobs -- 518 copies each, under 518 different and mostly wrong employers, 12.5% of
-- everything live search could find. Every crawl SUCCEEDED throughout. board_health stayed
-- green, the per-run family stayed green, and the queue depths stayed normal, because
-- nothing was failing: the platform answered 200 with valid postings. It ran for two weeks
-- and was found by a person reading the catalogue, not by a gauge.
--
-- The window is ten minutes, and that is a measured choice rather than a round number.
-- Measured on prod 2026-09-16: 24h costs 20,818ms, 60m costs 837ms, 15m costs 328ms, and
-- 10m costs 22ms -- the same order as ProviderIngestHealth's 54ms, which is the bar this
-- file's header sets. Ten minutes is also sufficient: the duplication appears BETWEEN
-- boards, never within one crawl (a board that fetches the global catalogue writes each of
-- its postings once), so the gap opens as soon as two boards of the same source land in one
-- window. apploi would have had roughly forty per window. A source whose boards crawl more
-- slowly than that reads 1:1 and is simply not accused.
--
-- Rides jobs_open_created_idx (created_at DESC, id DESC) WHERE closed_at IS NULL, which is
-- why it is scoped to open rows -- and that scope is honest for this question anyway: a row
-- closed within the window was withdrawn, not ingested.
--
-- Publishes the two RAW counts, never a ratio. Which ratio deserves a page belongs in the
-- alert rule, the same argument freehire_provider_boards already makes for board states --
-- and a ratio computed here would also have to invent an answer for a source that wrote
-- nothing, where the honest count is a pair of zeros.
--
-- Sources that write no url are excluded rather than counted as one big duplicate: a row
-- with no url cannot be told apart from another one, so including them would accuse a
-- source of duplication for a field it simply does not carry.
--
-- NOT is_private for the catalogue-wide reason (internal/job/privatejob), and because it is
-- right for this question on its own: a pasted JD was not INGESTED, and counting one would
-- put 'pasted' and 'weblink' in the exposition as providers that no crawl can explain.
SELECT
    j.source                        AS provider,
    count(*)::bigint                AS rows_written,
    count(DISTINCT j.url)::bigint   AS distinct_postings
FROM jobs j
WHERE j.closed_at IS NULL
  AND NOT j.is_private
  AND j.created_at > now() - interval '10 minutes'
  AND j.url <> ''
GROUP BY j.source
ORDER BY j.source;
