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

-- name: BoardHealthMetrics :one
-- The ingest board fleet split into three mutually exclusive states, so the published
-- gauges sum to the fleet size and a stacked graph reads correctly.
--
-- Cooled takes precedence over failing. A board in cooldown always has failures behind it
-- (see internal/ingest/pipeline's CooldownFor), so counting it in both states would double-count
-- every cooled board and make the total exceed the fleet. Cooled is the more actionable
-- of the two: it means the board is currently NOT being crawled.
SELECT
    count(*) FILTER (
        WHERE cooldown_until > now()
    ) AS cooled,
    count(*) FILTER (
        WHERE (cooldown_until IS NULL OR cooldown_until <= now())
          AND consecutive_failures > 0
    ) AS failing,
    count(*) FILTER (
        WHERE (cooldown_until IS NULL OR cooldown_until <= now())
          AND consecutive_failures = 0
    ) AS healthy
FROM board_health;

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
SELECT created_at
FROM jobs
WHERE closed_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: ProviderIngestHealth :many
-- Per-provider ingest health: when the provider's most recent board crawl succeeded, and
-- how its boards split across the same three states BoardHealthMetrics publishes fleet-wide.
--
-- The two halves answer different questions and neither substitutes for the other.
--
-- The timestamp says when data last ARRIVED. max() over a nullable column yields NULL for a
-- provider whose every board has never succeeded, and that NULL is rendered as an ABSENT
-- sample rather than a zero, because a Unix zero reads downstream as overdue since 1970.
-- So on the timestamp alone a provider is invisible in precisely the case where it is most
-- broken — gulftalent held 19,828 postings unrefreshed since 2026-07-07 with its systemd
-- timer disabled, and published no sample at all.
--
-- The state counts say whether anything is TRYING. Every provider board_health knows has a
-- row, so these always have a value, and "no healthy boards" is a predicate that fires for
-- the never-succeeded case as well as the stopped-succeeding one. It is also selective:
-- measured on prod 2026-09-01 it named 20 providers out of ~180, while a fleet whose
-- background is ~5% failing boards leaves a mostly-healthy provider like personio out.
--
-- The three states are mutually exclusive and carry BoardHealthMetrics' precedence rule
-- (cooled over failing), so they sum to the provider's board count and the fleet-wide
-- family stays the sum of these.
--
-- Reads board_health rather than the jobs table on purpose. The catalogue-side form of
-- the same question — max(last_seen_at) grouped by source over open jobs — measured 41s
-- and 2.1M buffer reads on prod (2026-09-01), a parallel sequential scan of 8M rows. This
-- file's header commits every query in it to never being why ingest waits, and the host
-- is disk-bound, so a once-a-minute scan of that size is exactly the thing it forbids.
-- The same measurement here reads 97k rows in 54ms.
SELECT
    provider,
    max(last_success_at)::timestamptz AS last_success_at,
    count(*) FILTER (
        WHERE cooldown_until > now()
    ) AS cooled,
    count(*) FILTER (
        WHERE (cooldown_until IS NULL OR cooldown_until <= now())
          AND consecutive_failures > 0
    ) AS failing,
    count(*) FILTER (
        WHERE (cooldown_until IS NULL OR cooldown_until <= now())
          AND consecutive_failures = 0
    ) AS healthy
FROM board_health
GROUP BY provider
ORDER BY provider;

-- name: NotifyBacklogMetrics :one
-- The subscription-digest backlog: how many active subscriptions have something
-- undelivered, and how old the oldest undelivered match is.
--
-- The age is the signal that matters. A pass runs every five minutes, so in steady state
-- the oldest pending match is minutes old. An age that climbs without bound means some
-- subscription is never being served — which is not visible in the worker's own log,
-- because a starved subscription produces no failure: `notify` reported
-- `delivered=1 failed=0` for weeks while 1.14M matches sat undelivered and one
-- subscription's had never been claimed at all (2026-09-04, see docs/agents/notifications.md).
--
-- Both COALESCE to 0 rather than staying NULL: a drained backlog is a real measurement
-- that must publish an explicit zero, because an absent series is how the consuming alert
-- rules recognize a dead exporter.
SELECT
    COALESCE(count(DISTINCT m.subscription_id), 0)::bigint AS pending_subscriptions,
    COALESCE(EXTRACT(EPOCH FROM now() - min(m.matched_at)), 0)::float8 AS oldest_age_seconds
FROM subscription_matches m
JOIN subscriptions s ON s.id = m.subscription_id
WHERE m.notified_at IS NULL AND m.failed_at IS NULL AND s.active;
