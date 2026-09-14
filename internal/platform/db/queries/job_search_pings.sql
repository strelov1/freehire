-- name: ListJobsToPing :many
-- The newest eligible postings this engine has not been told about yet, for
-- cmd/search-ping. The gate mirrors needsRecentFeed in cmd/ingest/store.go — open,
-- canonical, public, technical — because a URL worth announcing is a URL the site
-- itself claims, and those are the same postings.
--
-- NEWEST FIRST is the whole selection policy, and it is doing two jobs. The obvious
-- one: a posting is most worth announcing while it is still open, and the budget is
-- far smaller than the catalogue, so anything but recency spends it on pages whose
-- moment has passed. The second is a quality guard we get for free — the sitemap
-- additionally excludes the "likely-evergreen" reality class, which lives only in the
-- search index and cannot be joined here, but that class is earned by a posting
-- staying open for a long time, so the newest rows have not had the chance to qualify.
-- Google adjusts the daily quota by the quality of what is submitted, which is why the
-- divergence is worth naming rather than leaving to be discovered.
--
-- The anti-join is served by job_search_pings_pkey; the ORDER BY by the same
-- open-and-recent index the public feed uses.
SELECT j.id, j.public_slug
FROM jobs j
WHERE j.closed_at IS NULL
  AND j.duplicate_of IS NULL
  AND NOT j.is_private
  AND j.is_tech IS TRUE
  AND j.public_slug IS NOT NULL
  AND NOT EXISTS (
      SELECT 1
      FROM job_search_pings p
      WHERE p.job_id = j.id
        AND p.engine = sqlc.arg(engine)
  )
ORDER BY j.created_at DESC
LIMIT sqlc.arg(batch_size);

-- name: RecordJobSearchPing :exec
-- Record that this posting was announced to this engine. ON CONFLICT DO NOTHING keeps
-- a re-run after a partial failure from double-spending a budget that is counted in
-- hundreds per day: the row is what makes the send idempotent, so it is written per
-- URL as each send succeeds rather than once for the batch at the end.
INSERT INTO job_search_pings (job_id, engine)
VALUES (sqlc.arg(job_id), sqlc.arg(engine))
ON CONFLICT (job_id, engine) DO NOTHING;

-- name: CountJobSearchPingsSince :one
-- How many URLs this engine has been sent since a moment — what the worker logs, and
-- the only way to see a bounded daily budget actually being spent. Served by
-- job_search_pings_engine_pinged_at_idx.
SELECT count(*)
FROM job_search_pings
WHERE engine = sqlc.arg(engine)
  AND pinged_at >= sqlc.arg(since);
