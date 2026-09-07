-- name: EnqueueRecentFeedOutbox :exec
-- Queue a job for the homepage's live "recently added" feed. Called by cmd/ingest
-- inside the same transaction as the job's upsert, only for a canonical, non-duplicate,
-- IT/tech posting (see cmd/ingest/store.go). ON CONFLICT keeps at most one live entry
-- per job — this is a pure transit queue with no lease/retry bookkeeping, drained
-- within seconds by the poller in internal/job/recentfeed, so there is nothing to
-- reconcile beyond "don't queue the same job twice while it's still pending".
INSERT INTO recent_feed_outbox (job_id)
VALUES (sqlc.arg(job_id))
ON CONFLICT (job_id) DO NOTHING;

-- name: ClaimRecentFeedOutboxBatch :many
-- Claim-and-delete a bounded batch, oldest first, joined to jobs for the fields the
-- feed displays. Unlike search_outbox/enrichment_outbox there is no lease: a claimed
-- row is deleted outright in the same statement, because a cosmetic feed has nothing
-- to retry and nothing to reconcile if the connection holding it dies mid-drain — the
-- row is simply gone either way, and the next poll tick picks up whatever else queued.
--
-- FOR UPDATE SKIP LOCKED on the id-selecting subquery lets concurrent callers (there is
-- normally only one poller, but this keeps the statement safe if that ever changes)
-- take disjoint rows instead of blocking on each other.
WITH claimed AS (
    DELETE FROM recent_feed_outbox
    WHERE job_id IN (
        SELECT job_id
        FROM recent_feed_outbox
        ORDER BY created_at ASC
        LIMIT sqlc.arg(batch_size)
        FOR UPDATE SKIP LOCKED
    )
    RETURNING job_id
)
SELECT j.id AS job_id, j.title, j.company, j.public_slug
FROM claimed c
JOIN jobs j ON j.id = c.job_id;
