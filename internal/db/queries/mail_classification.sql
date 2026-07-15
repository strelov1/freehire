-- name: EnqueuePendingEmailClassification :execrows
-- Idempotent backfill: enqueue every email not yet classified. classified_at is the
-- "done" marker; ON CONFLICT keeps one entry per email, so running this each worker
-- invocation never duplicates work.
INSERT INTO email_classification_outbox (email_id)
SELECT id FROM emails WHERE classified_at IS NULL
ON CONFLICT (email_id) DO NOTHING;

-- name: ClaimEmailClassificationBatch :many
-- Claim a wave of live, unleased entries by stamping claimed_at, newest email first,
-- returning the email fields the matcher/classifier need. FOR UPDATE OF o locks only
-- outbox rows; SKIP LOCKED lets concurrent workers take disjoint rows; the lease
-- predicate reclaims entries whose worker died, so no separate reaper is needed.
WITH claimable AS (
    SELECT o.id, o.email_id
    FROM email_classification_outbox o
    JOIN emails e ON e.id = o.email_id
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
    ORDER BY e.received_at DESC, e.id DESC
    FOR UPDATE OF o SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE email_classification_outbox o
SET claimed_at = now()
FROM claimable c
JOIN emails e ON e.id = c.email_id
WHERE o.id = c.id
RETURNING o.id, o.email_id, e.user_id, e.thread_id, e.from_name, e.subject, e.body_text, e.body_html;

-- name: SetEmailClassification :exec
-- Persist the resolved link + classification and stamp classified_at + model in one
-- write. job_id/suggested_job_id/link_source/match_confidence are nullable — an
-- unlinked or suggestion-only email leaves job_id NULL.
UPDATE emails
SET job_id               = sqlc.narg(job_id),
    suggested_job_id     = sqlc.narg(suggested_job_id),
    link_source          = sqlc.narg(link_source),
    match_confidence     = sqlc.narg(match_confidence),
    status_signal        = sqlc.narg(status_signal),
    classification_model = sqlc.arg(model),
    classified_at        = now()
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: DeleteEmailClassificationOutbox :exec
DELETE FROM email_classification_outbox WHERE id = $1;

-- name: FailEmailClassification :exec
-- Record a failed attempt: bump attempts, release the lease, store the error, and
-- dead-letter (set failed_at) once attempts reach max_attempts.
UPDATE email_classification_outbox
SET attempts    = attempts + 1,
    claimed_at  = NULL,
    last_error  = sqlc.arg(last_error),
    failed_at   = CASE WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now() ELSE NULL END
WHERE id = sqlc.arg(id);

-- name: ListUserApplicationsForMatch :many
-- The caller's open applications offered to the matcher (applied, saved, or staged),
-- as (job_id, company). Closed postings are excluded.
SELECT j.id, j.company
FROM user_jobs uj
JOIN jobs j ON j.id = uj.job_id
WHERE uj.user_id = $1
  AND j.closed_at IS NULL
  AND (uj.applied_at IS NOT NULL OR uj.saved_at IS NOT NULL OR uj.stage IS NOT NULL);

-- name: ListUserEmailThreadLinks :many
-- Existing thread→application links for the caller, so the matcher can continue a
-- thread already attached to an application.
SELECT thread_id, job_id
FROM emails
WHERE user_id = $1 AND job_id IS NOT NULL AND thread_id <> '';

-- name: GetUserJobStage :one
-- The caller's current stage for one application (empty string when unset), so the
-- worker can decide a monotonic-forward advancement.
SELECT COALESCE(stage, '')::text AS stage
FROM user_jobs
WHERE user_id = $1 AND job_id = $2;

-- name: AdvanceUserJobStage :exec
-- Move an application forward to a new stage (the worker only calls this after
-- checking the transition is strictly forward and high-confidence).
UPDATE user_jobs SET stage = $3 WHERE user_id = $1 AND job_id = $2;
