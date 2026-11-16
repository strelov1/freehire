-- name: LockJobForEnrichmentCompletion :one
-- Lock the posting before checking the claim in a SECOND statement. If an
-- ownership transfer completed while waiting, that second READ COMMITTED snapshot
-- must see its invalidated queue entry rather than a pre-lock snapshot.
SELECT id FROM jobs WHERE id = $1 FOR NO KEY UPDATE;

-- name: HasEnrichmentCompletionClaim :one
SELECT EXISTS (
    SELECT 1 FROM enrichment_outbox
    WHERE id = sqlc.arg(outbox_id)::bigint
      AND job_id = sqlc.arg(job_id)::bigint
      AND target_version = sqlc.arg(target_version)::int
);
