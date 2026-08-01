-- name: EnqueuePendingEmailClassification :execrows
-- Idempotent backfill: enqueue every email not yet classified. classified_at is the
-- "done" marker; ON CONFLICT keeps one entry per email, so running this each worker
-- invocation never duplicates work.
--
-- 'external' mail is excluded: it was pushed by the caller's own harness, which
-- brings its own classifier, so classifying it here would spend our LLM tokens on
-- the one tier that is meant to cost us nothing. Such mail stays unclassified
-- indefinitely by design — the agent finds its backlog via the inbox listing's
-- unclassified filter.
INSERT INTO email_classification_outbox (email_id)
SELECT id FROM emails WHERE classified_at IS NULL AND source <> 'external'
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
RETURNING o.id, o.email_id, e.user_id, e.source, e.thread_id, e.from_addr, e.from_name, e.subject, e.body_text, e.body_html;

-- name: SetEmailClassification :exec
-- Persist the resolved link + classification and stamp classified_at + model in one
-- write. job_id/suggested_job_id/link_source/match_confidence are nullable — an
-- unlinked or suggestion-only email leaves job_id NULL.
UPDATE emails
SET job_id               = sqlc.narg(job_id),
    -- Kept in step with job_id, and derived rather than passed: the caller names a
    -- posting, and the application is what the ledger and the aggregates pair on. Left
    -- NULL when the user has no application for that posting — an unlinked fact is
    -- useful, a wrongly attached one is not.
    application_id       = (SELECT a.id FROM applications a
                             WHERE a.user_id = emails.user_id AND a.job_id = sqlc.narg(job_id)),
    suggested_job_id     = sqlc.narg(suggested_job_id),
    link_source          = sqlc.narg(link_source),
    match_confidence     = sqlc.narg(match_confidence),
    status_signal        = sqlc.narg(status_signal),
    classification_model = sqlc.arg(model),
    classified_at        = now()
WHERE emails.id = sqlc.arg(id) AND emails.user_id = sqlc.arg(user_id);

-- name: AgentTriageEmail :execrows
-- Persist an agent-produced verdict for one message, scoped to the caller (0 rows
-- when the message is not theirs → 404). This is SetEmailClassification's sibling:
-- the same columns, written in one update, so a message is never left classified
-- but unstamped or linked but unclassified.
--
-- A NULL job_id means "I am not deciding the link" — the existing link and its
-- provenance are kept. Clearing a link stays the explicit UnlinkEmail action, so a
-- classify-only triage can never silently detach an application. Any pending
-- suggestion is dropped either way: the agent's verdict supersedes it.
--
-- match_confidence belongs to the LINK, not to the classification, so it follows
-- job_id rather than being overwritten on every call: a stated confidence wins; a
-- NEW link with none stated clears it (nobody said how sure they were about THIS
-- link); an untouched link keeps the confidence it was made with. Writing the
-- argument unconditionally left rows reading link_source='agent' with a NULL
-- confidence after a caller merely re-labelled the message.
UPDATE emails
SET status_signal        = sqlc.narg(status_signal),
    job_id               = COALESCE(sqlc.narg(job_id), job_id),
    -- Follows job_id, derived rather than passed, exactly as the other link paths do:
    -- an untouched link keeps the application it already names.
    application_id       = CASE WHEN sqlc.narg(job_id) IS NOT NULL
                                THEN (SELECT a.id FROM applications a
                                       WHERE a.user_id = emails.user_id
                                         AND a.job_id = sqlc.narg(job_id))
                                ELSE application_id END,
    link_source          = CASE WHEN sqlc.narg(job_id) IS NOT NULL THEN 'agent' ELSE link_source END,
    -- The cast is load-bearing: this is the parameter's FIRST appearance, and it is
    -- inside an IS NOT NULL, which tells Postgres nothing about its type. Without
    -- it the statement fails to plan with "could not determine data type". job_id
    -- above needs no cast only because COALESCE anchors it a line earlier.
    match_confidence     = CASE
                               WHEN sqlc.narg(confidence)::real IS NOT NULL THEN sqlc.narg(confidence)::real
                               WHEN sqlc.narg(job_id) IS NOT NULL THEN NULL
                               ELSE match_confidence
                           END,
    suggested_job_id     = NULL,
    classification_model = 'agent',
    classified_at        = now()
WHERE emails.id = sqlc.arg(id) AND emails.user_id = sqlc.arg(user_id);

-- name: DeleteEmailClassificationOutbox :exec
DELETE FROM email_classification_outbox WHERE id = $1;

-- name: FailEmailClassification :one
-- Record a failed attempt: bump attempts, store the error, and dead-letter (set
-- failed_at) once attempts reach max_attempts. The lease (claimed_at) is
-- intentionally left in place — its expiry gates the retry to a later run and
-- doubles as the crash reaper, so a failed entry is never reprocessed within the
-- same run. Mirrors RecordEnrichmentFailure / RecordSemanticFailure, RETURNING included:
-- failed_at is how the caller learns an entry dead-lettered, which is what decides the
-- worker's exit code. Without it a mail queue can dead-letter every entry and still exit 0.
UPDATE email_classification_outbox
SET attempts    = attempts + 1,
    last_error  = sqlc.arg(last_error),
    failed_at   = CASE WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now() ELSE NULL END
WHERE id = sqlc.arg(id)
RETURNING attempts, failed_at;

-- name: ListUserApplicationsForMatch :many
-- The caller's open applications offered to the matcher (applied, saved, or staged),
-- as (job_id, company). Closed postings are excluded.
SELECT j.id, j.company
FROM user_jobs uj
JOIN jobs j ON j.id = uj.job_id
LEFT JOIN applications a ON a.user_id = uj.user_id AND a.job_id = uj.job_id
WHERE uj.user_id = $1
  AND j.closed_at IS NULL
  AND (a.applied_at IS NOT NULL OR uj.saved_at IS NOT NULL OR a.stage IS NOT NULL);

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
FROM applications
WHERE user_id = sqlc.arg(user_id)::bigint AND job_id = sqlc.arg(job_id)::bigint;

-- name: AdvanceUserJobStage :exec
-- Move an application forward to a new stage (the worker only calls this after
-- checking the transition is strictly forward and high-confidence).
UPDATE applications SET stage = sqlc.arg(stage)::text
 WHERE user_id = sqlc.arg(user_id)::bigint AND job_id = sqlc.arg(job_id)::bigint;
