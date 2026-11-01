-- name: UpsertApplyForm :exec
-- Store a job's captured application form, replacing whatever was there. job_id is the
-- primary key, so this is the only shape a write can take: a job carries one current
-- form, and a re-capture supersedes rather than accumulates. captured_at is refreshed on
-- every write so a form's age always describes the payload actually stored.
--
-- A form that is new or different also queues the job for the live search index, in the
-- same statement, because the search document carries the form's question count
-- (`apply_questions`, counted from this payload at index time) and nothing else would
-- carry the change there: cmd/capture-apply-form fills a form hours or days after
-- cmd/ingest queued the job's document, and an employer saves a vacancy's questions after
-- the vacancy itself. Without this the count reached Meilisearch only at the next full
-- rebuild. In the statement rather than in each writer so no writer can forget it — there
-- are three.
--
-- `previous` reads the row as it stood before this statement (every CTE shares one
-- snapshot), so a Recruitee crawl rewriting the identical form on every run queues
-- nothing. Only an open, canonical, public job is queued — the same rows
-- ClaimSearchOutboxBatch would drain; anything else is a row the drain would claim and
-- discard. The form is still stored for those, so a posting that becomes canonical later
-- is indexed with its count by the write that makes it so.
WITH previous AS (
    SELECT f.payload FROM apply_forms f WHERE f.job_id = sqlc.arg(job_id)::bigint
),
stored AS (
    INSERT INTO apply_forms (job_id, provider, payload)
    VALUES (sqlc.arg(job_id), sqlc.arg(provider), sqlc.arg(payload))
    ON CONFLICT (job_id) DO UPDATE
    SET provider    = EXCLUDED.provider,
        payload     = EXCLUDED.payload,
        captured_at = now()
    RETURNING apply_forms.job_id
)
INSERT INTO search_outbox (job_id, job_posted_at)
SELECT j.id, COALESCE(j.posted_at, j.created_at)
FROM stored s
JOIN jobs j ON j.id = s.job_id
WHERE j.closed_at IS NULL
  AND j.duplicate_of IS NULL
  AND NOT j.is_private
  AND (SELECT payload FROM previous) IS DISTINCT FROM sqlc.arg(payload)::jsonb
-- An entry already pending is RE-STAMPED rather than left alone, unlike
-- EnqueueSearchOutbox's DO NOTHING. cmd/reindex clears every entry older than its run once
-- the rebuild is promoted (DeleteSearchOutboxCreatedBefore), trusting jobs.updated_at to
-- catch a job that changed again during the run — and this write does not touch jobs, so
-- an old entry for a job whose form arrived after the rebuild read it would be cleared
-- with the new count never indexed. Moving created_at is what marks it as newer.
-- A changed form also replaces the entry's identity. A drain that already claimed the
-- old id can finish (or fail) only that old generation, leaving this update pending.
-- Keep its lease so a second drain cannot overtake the in-flight index write.
ON CONFLICT (job_id) DO UPDATE SET id = DEFAULT, created_at = now();

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
-- The claim returns the job's source, external_id and url because the worker builds its
-- fetch from the row alone: external_id is the board-namespaced posting id
-- (externalid.Namespace), which carries both halves the platform APIs need, and
-- the url carries the regional host for a platform that has more than one. Lever serves
-- its European tenants from a separate host and answers 404 on the other — the same code
-- it uses for a posting that is gone, so the host cannot be discovered by trying.
--
-- skip_sources names the platforms that answered 429 earlier in this run. Their entries
-- stay queued untouched for a later run; leaving them claimable here would hand every
-- following wave the same throttled platform's freshest postings and starve the rest.
-- COALESCE because `<> ALL(NULL)` matches no row at all, and a nil Go slice arrives as
-- NULL: without it, a caller that skips nothing would claim nothing.
WITH claimable AS (
    SELECT o.id, o.job_id
    FROM apply_form_outbox o
    JOIN jobs j ON j.id = o.job_id
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
      AND j.source <> ALL(COALESCE(sqlc.arg(skip_sources)::text[], '{}'))
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
RETURNING o.id, o.job_id, j.source, j.external_id, j.url;

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

-- name: ListApplyFormsForJobs :many
-- The captured forms of a BATCH of jobs, for the search-index writers that count each
-- form's questions onto its document (applyform.QuestionCounts). One call per writer
-- batch, never one per document — the same shape ListSemanticChunksForJobs gives the
-- description vectors. A job with no captured form simply has no row.
SELECT job_id, provider, payload
FROM apply_forms
WHERE job_id = ANY(sqlc.arg(job_ids)::bigint[]);
