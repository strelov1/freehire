-- name: UpsertApplyForm :exec
-- Store a job's captured application form, replacing whatever was there. job_id is the
-- primary key, so this is the only shape a write can take: a job carries one current
-- form, and a re-capture supersedes rather than accumulates. captured_at is refreshed on
-- every write so a form's age always describes the payload actually stored.
INSERT INTO apply_forms (job_id, provider, payload)
VALUES (sqlc.arg(job_id), sqlc.arg(provider), sqlc.arg(payload))
ON CONFLICT (job_id) DO UPDATE
SET provider    = EXCLUDED.provider,
    payload     = EXCLUDED.payload,
    captured_at = now();

-- name: EnqueueApplyFormCapture :execrows
-- Transactional-outbox enqueue for the ingest write path: queue this one job for a form
-- capture, gated on it having none.
--
-- The gate is deliberately "has no form" rather than the content-hash staleness gate the
-- semantic queue uses. A posting's description is reworded often; its application form is
-- configured once on the requisition and then sits still. Gating on content change would
-- re-fetch a form every time a company edited its job ad — 135k Greenhouse fetches for a
-- payload that did not move. Refreshing a capture is therefore a deliberate act (drop the
-- stored row, which reopens this gate), not something a crawl does by accident.
--
-- Idempotent via the outbox's UNIQUE (job_id). Run in the same transaction as the job's
-- UpsertJob so a newly ingested job is queued atomically with its write.
INSERT INTO apply_form_outbox (job_id)
SELECT j.id
FROM jobs j
WHERE j.id = sqlc.arg(job_id)::bigint
  AND NOT EXISTS (SELECT 1 FROM apply_forms f WHERE f.job_id = j.id)
ON CONFLICT (job_id) DO NOTHING;

-- name: ClaimApplyFormBatch :many
-- Claim a batch of live, unleased captures, freshest posting first, by stamping
-- claimed_at. Mirrors ClaimSemanticBatch: FOR UPDATE OF o locks only outbox rows (a bare
-- FOR UPDATE would also lock jobs, making concurrent claim waves contend), SKIP LOCKED
-- lets concurrent workers take disjoint rows, and the lease predicate reclaims entries
-- whose worker died, so no separate reaper process is needed.
--
-- Freshness is COALESCE(posted_at, created_at) so postings without a source date fall
-- back to ingest time instead of starving under NULLS LAST. It matters less here than for
-- embeddings — a form does not go stale — but a just-posted job is the one somebody is
-- about to apply to, so it is still the right order.
--
-- The claim returns the job's source and external_id because the worker builds its fetch
-- from the row alone: external_id is the board-namespaced posting id
-- (sources.NamespaceExternalID), which carries both halves the platform APIs need.
WITH claimable AS (
    SELECT o.id, o.job_id
    FROM apply_form_outbox o
    JOIN jobs j ON j.id = o.job_id
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
    ORDER BY COALESCE(j.posted_at, j.created_at) DESC, j.id DESC
    FOR UPDATE OF o SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE apply_form_outbox o
SET claimed_at = now()
-- Join jobs off the claimable CTE (not the UPDATE target o, which Postgres forbids in
-- FROM) so the platform identity comes back without a second query.
FROM claimable c
JOIN jobs j ON j.id = c.job_id
WHERE o.id = c.id
RETURNING o.id, o.job_id, j.source, j.external_id;

-- name: DeleteApplyFormEntry :exec
-- Retire a capture that succeeded. The stored form is the record; the queue entry has
-- nothing left to say.
DELETE FROM apply_form_outbox
WHERE id = sqlc.arg(id);

-- name: RecordApplyFormFailure :one
-- Count a failed capture: bump attempts, record the error, and dead-letter (set
-- failed_at) once attempts reach the max. The lease (claimed_at) is intentionally left in
-- place — its expiry gates the retry to a later run and doubles as the crash reaper, so a
-- failed entry is never reprocessed within the same run. Mirrors RecordSemanticFailure.
UPDATE apply_form_outbox
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now()
                     ELSE NULL
                 END
WHERE id = sqlc.arg(id)
RETURNING attempts, failed_at;

-- name: GetApplyFormByJobID :one
-- Read one job's captured form for display. The only read path over this store, and it
-- is by primary key — the display surface asks for exactly one posting's form, never a
-- page of them, which is also why nothing here joins jobs.
SELECT provider, captured_at, payload
FROM apply_forms
WHERE job_id = sqlc.arg(job_id);
