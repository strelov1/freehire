-- name: ClaimAutoApplyBatch :many
-- Claim a batch of live, unleased submission attempts by stamping claimed_at. Mirrors
-- ClaimApplyFormBatch: FOR UPDATE OF q locks only queue rows (a bare FOR UPDATE would also
-- lock jobs, making concurrent claim waves contend), SKIP LOCKED lets concurrent workers
-- take disjoint rows, and the lease predicate reclaims entries whose worker died, so no
-- separate reaper process is needed. failed_at/blocked_at excluded so a dead-lettered or
-- parked attempt is never reclaimed by this query — auto_apply_queue_claimable_idx exists
-- for exactly this predicate.
--
-- tailored_cv_id IS NOT NULL AND review_decision = 'approved' (openspec/changes/
-- auto-apply-tailored-resume): a submission attempt only ever runs for an entry the
-- candidate has reviewed and approved. An unreviewed or declined entry sits in the queue
-- but is never claimed — declining also sets blocked_at (via the review endpoint's own
-- Park-shaped write), so it is additionally excluded by the predicate above.
--
-- Returns job.source, job.external_id and job.url because the caller builds the sidecar
-- request from the row alone — source doubles as the ATS provider name, the same vocabulary
-- internal/applyform's Provider field already uses, and external_id (board:posting-id) is
-- what internal/applyform's own schema fetchers need to reuse their existing per-provider
-- API calls rather than re-deriving them.
WITH claimable AS (
    SELECT q.id, q.user_id, q.job_id
    FROM auto_apply_queue q
    WHERE q.failed_at IS NULL
      AND q.blocked_at IS NULL
      AND q.tailored_cv_id IS NOT NULL
      AND q.review_decision = 'approved'
      AND (q.claimed_at IS NULL
           OR q.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
    ORDER BY q.id
    FOR UPDATE OF q SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE auto_apply_queue q
SET claimed_at = now()
FROM claimable c
JOIN jobs j ON j.id = c.job_id
WHERE q.id = c.id
RETURNING q.id, q.user_id, q.job_id, q.tailored_cv_id, j.source, j.external_id, j.url;

-- name: DeleteAutoApplyEntry :exec
-- Retire an attempt that submitted successfully. jobtracking's MarkJobApplied (called in
-- the same transaction, alongside LockJobForApply) is the durable record; the queue entry
-- has nothing left to say. Mirrors DeleteApplyFormEntry.
DELETE FROM auto_apply_queue
WHERE id = sqlc.arg(id);

-- name: MarkAutoApplyBlocked :exec
-- Park an attempt the sidecar could not fully resolve: record which fields stopped it and
-- why, and leave the lease in place. Unlike RecordAutoApplyFailure this is not a retry
-- countdown — a parked attempt needs new data, not another try — so attempts is left
-- untouched and blocked_at, not failed_at, is what excludes it from
-- auto_apply_queue_claimable_idx from here on.
UPDATE auto_apply_queue
SET blocked_at = now(),
    last_error = sqlc.arg(last_error),
    unmapped   = sqlc.arg(unmapped)
WHERE id = sqlc.arg(id);

-- name: RecordAutoApplyFailure :one
-- Count a transient failure: bump attempts, record the error, and dead-letter (set
-- failed_at) once attempts reach the max. The lease (claimed_at) is intentionally left in
-- place — its expiry gates the retry to a later run, so a failed entry is never
-- reprocessed within the same run. Mirrors RecordApplyFormFailure.
UPDATE auto_apply_queue
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now()
                     ELSE NULL
                 END
WHERE id = sqlc.arg(id)
RETURNING attempts, failed_at;

-- name: GetAutoApplyQueueEntryForReview :one
-- One read backing both the tailoring-trigger and the review-decision endpoints
-- (openspec/changes/auto-apply-tailored-resume): resolves ownership (a foreign or missing
-- id comes back as pgx.ErrNoRows, which the handler renders as 404 — never 403, so a
-- probing caller learns nothing about entries they do not own) and carries enough of the
-- entry's own state (job_id for tailoring, tailored_cv_id/review_decision for the review
-- gate) that neither endpoint needs a second query to decide whether to proceed.
SELECT id, user_id, job_id, tailored_cv_id, review_decision
FROM auto_apply_queue
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: GetAutoApplyQueueEntryByID :one
-- The same read as GetAutoApplyQueueEntryForReview, but by id ALONE — no ownership
-- predicate. This is for the trusted auto-apply orchestrator caller only
-- (openspec/changes/auto-apply-inngest-orchestration): it authenticates as the
-- deployment's own shared secret, not as any particular user, so it has no owner of its
-- own to check the row against — the row's own user_id in the result IS the owner it
-- acts as. Never used for a caller that presented ownership-scoped credentials; see
-- resolveAutoApplyEntry in internal/api/handler/auto_apply_tailor.go.
SELECT id, user_id, job_id, tailored_cv_id, review_decision
FROM auto_apply_queue
WHERE id = sqlc.arg(id);

-- name: SetAutoApplyTailoredCV :execrows
-- Records which tailored CV a queue entry's tailoring run produced. Guarded by
-- review_decision IS NULL, matching ApproveAutoApplyReview/DeclineAutoApplyReview's own
-- guard: PostAutoApplyTailor's own review_decision check happens before its (potentially
-- minutes-long) LLM run, not after, so a candidate can approve or decline an EARLIER
-- tailored CV while a stale or retried tailor call for the same entry is still in flight.
-- Without this guard that call's own write here would silently attach a fresh, never-seen
-- CV to an already-decided entry — which ClaimAutoApplyBatch's own predicate
-- (tailored_cv_id IS NOT NULL AND review_decision = 'approved') would then submit for
-- real. Zero rows here means exactly that race happened; the handler checks it.
UPDATE auto_apply_queue
SET tailored_cv_id = sqlc.arg(tailored_cv_id)
WHERE id = sqlc.arg(id) AND review_decision IS NULL;

-- name: ApproveAutoApplyReview :execrows
-- Records an approval. Guarded by review_decision IS NULL so a second attempt at an
-- already-reviewed entry affects zero rows rather than overwriting a recorded decision —
-- the handler reads the row first (GetAutoApplyQueueEntryForReview) to tell "already
-- reviewed" apart from "not found" before ever reaching this statement, so zero rows here
-- would only mean a race with a concurrent decision on the same entry.
UPDATE auto_apply_queue
SET review_decision = 'approved',
    reviewed_at     = now()
WHERE id = sqlc.arg(id) AND review_decision IS NULL;

-- name: EnqueueAutoApply :one
-- Creates the candidate-facing entry that starts the tailor-then-review sequence
-- (openspec/changes/auto-apply-submit-trigger). ON CONFLICT DO NOTHING against the
-- existing UNIQUE (user_id, job_id) (migration 0116) rather than a SELECT-then-INSERT: a
-- double-click or a page reload racing this same request is expected, common traffic here,
-- not a fault, and the constraint is the only thing that closes the window between a
-- check and an insert. No row back means the handler's own INSERT lost the race (or the
-- pair was already queued by an earlier request) — it re-reads via
-- GetAutoApplyQueueEntryForJob rather than treating an empty result as an error.
INSERT INTO auto_apply_queue (user_id, job_id)
VALUES (sqlc.arg(user_id), sqlc.arg(job_id))
ON CONFLICT (user_id, job_id) DO NOTHING
RETURNING id;

-- name: GetAutoApplyQueueEntryForJob :one
-- The caller's own existing auto-apply entry for one job, if any — the conflict-read path
-- for EnqueueAutoApply, and the read behind the job detail response's own auto-apply
-- status field (openspec/changes/auto-apply-submit-trigger). review_decision distinguishes
-- a live, undecided entry from a permanently declined one; pgx.ErrNoRows means no attempt
-- exists yet for this (user, job) pair.
--
-- failed_at/blocked_at are also read (a code review found their absence): once
-- cmd/auto-apply claims an approved entry, a dead-letter (RecordAutoApplyFailure) or a
-- form-field park (MarkAutoApplyBlocked) leaves review_decision at 'approved' — without
-- these two columns, both call sites would read a permanently stuck submission as
-- indistinguishable from a healthy one still in flight.
SELECT id, review_decision, failed_at, blocked_at
FROM auto_apply_queue
WHERE user_id = sqlc.arg(user_id) AND job_id = sqlc.arg(job_id);

-- name: DeclineAutoApplyReview :execrows
-- Records a decline AND parks the entry in one statement — the same fields
-- MarkAutoApplyBlocked sets (blocked_at, last_error), reusing that park vocabulary rather
-- than inventing a second one, plus the review columns MarkAutoApplyBlocked has no reason
-- to know about. unmapped stays NULL: this is not a form-field park. last_error is what
-- tells the two park reasons apart in the queue's own history, per design.md.
UPDATE auto_apply_queue
SET review_decision = 'declined',
    reviewed_at     = now(),
    blocked_at      = now(),
    last_error      = sqlc.arg(last_error)
WHERE id = sqlc.arg(id) AND review_decision IS NULL;
