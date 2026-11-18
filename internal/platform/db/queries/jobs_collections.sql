-- name: PropagateCollectionsToJobsBatch :one
-- Denormalize each company's curated-collection set onto its jobs, so the search
-- facet (jobs.collections) reflects current membership — one keyset batch at a
-- time. Run by cmd/import-collections after it writes companies.collections, which
-- loops until a batch updates nothing. updated_at is bumped so `reindex --since`
-- picks the changed rows up; the IS DISTINCT FROM guard skips unchanged rows, making
-- re-runs idempotent and cheap.
--
-- Batched because it is no longer safe as one statement: a credential reaching a
-- few thousand more companies rewrites hundreds of thousands of jobs, and a single
-- UPDATE holding all those row locks at once deadlocked against the live ingest
-- writers on 2026-10-21. A batch locks at most batch_size rows, in id order, and is
-- its own transaction, so a deadlock costs one batch's retry rather than the run.
--
-- Keyset on id rather than an id-range walk: ids are sparse (12M rows over a 1.6B
-- span), so fixed ranges would be tens of thousands of mostly-empty statements.
-- last_id is the cursor for the next batch; updated = 0 means nothing past
-- after_id diverges and the pass is done.
WITH batch AS (
    SELECT j.id, c.collections
    FROM jobs j
    JOIN companies c ON c.slug = j.company_slug
    WHERE j.id > sqlc.arg(after_id)::bigint
      AND j.collections IS DISTINCT FROM c.collections
    ORDER BY j.id
    LIMIT sqlc.arg(batch_size)::int
), updated AS (
    UPDATE jobs
    SET collections = batch.collections,
        updated_at  = now()
    FROM batch
    WHERE jobs.id = batch.id
    RETURNING jobs.id
)
SELECT count(*)::bigint AS updated,
       COALESCE(max(id), 0)::bigint AS last_id
FROM updated;
