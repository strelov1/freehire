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

-- name: ProviderIngestFreshness :many
-- The most recent successful crawl per ingest provider, for the gauge that makes a
-- provider which has stopped producing data visible as itself.
--
-- Reads board_health rather than the jobs table on purpose. The catalogue-side form of
-- the same question — max(last_seen_at) grouped by source over open jobs — measured 41s
-- and 2.1M buffer reads on prod (2026-09-01), a parallel sequential scan of 8M rows. This
-- file's header commits every query in it to never being why ingest waits, and the host
-- is disk-bound, so a once-a-minute scan of that size is exactly the thing it forbids.
-- The same measurement here reads 97k rows in 54ms.
--
-- max() over a nullable column yields NULL for a provider whose every board has never
-- succeeded; that NULL is carried through and rendered as an ABSENT sample, never as a
-- zero — see cmd/queue-metrics/render.go.
SELECT provider, max(last_success_at)::timestamptz AS last_success_at
FROM board_health
GROUP BY provider
ORDER BY provider;
