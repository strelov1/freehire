-- name: UpsertDirectApplication :one
-- Records a candidate's application to an on-site vacancy and CLAIMS it for delivery
-- (sending_at), or replaces the content of one they recorded earlier that never reached the
-- employer — the retry after a failed delivery. See migration 0187. The CV file it sends is
-- copied into direct_application_cvs in the same statement (migration 0270), so the copy
-- the employer's inbox serves is always the one this attempt mails, never a stale one.
--
-- Returns no row in exactly three cases, which GetDirectApplication tells apart afterwards:
-- the application was already SENT; another request holds a live claim on it (a delivery in
-- flight — claiming it again would email the employer twice); or the candidate has reached
-- today's cap. The cap counts applications created since midnight UTC
-- on OTHER vacancies, and a retry on this one always passes, so a failed delivery never
-- costs a second slot. Two concurrent first applications can both pass the count and land
-- one over the cap; the cap bounds abuse, it is not a quota anyone is billed against.
WITH app AS (
INSERT INTO direct_applications (job_id, user_id, cv_id, cv_kind, note, answers, sending_at)
SELECT @job_id, @user_id, @cv_id, @cv_kind, @note, @answers, now()
WHERE EXISTS (
        SELECT 1 FROM direct_applications
        WHERE job_id = @job_id AND user_id = @user_id
    )
   OR (
        SELECT count(*) FROM direct_applications
        WHERE user_id = @user_id
          AND job_id <> @job_id
          AND created_at >= date_trunc('day', now(), 'UTC')
    ) < sqlc.arg(daily_limit)::bigint
ON CONFLICT (job_id, user_id) DO UPDATE
SET cv_id = EXCLUDED.cv_id, cv_kind = EXCLUDED.cv_kind, note = EXCLUDED.note, answers = EXCLUDED.answers, sending_at = now()
WHERE direct_applications.sent_at IS NULL
  AND (direct_applications.sending_at IS NULL
       OR direct_applications.sending_at < now() - interval '2 minutes')
RETURNING id
), cv AS (
    INSERT INTO direct_application_cvs (application_id, filename, content_type, content)
    SELECT app.id, @cv_filename, @cv_content_type, @cv_content FROM app
    ON CONFLICT (application_id) DO UPDATE
    SET filename = EXCLUDED.filename, content_type = EXCLUDED.content_type, content = EXCLUDED.content
)
SELECT app.id FROM app;

-- name: MarkDirectApplicationSent :exec
-- Stamps an application delivered. Only ever after the email succeeded.
UPDATE direct_applications
SET sent_at = now(), sending_at = NULL
WHERE id = $1 AND sent_at IS NULL;

-- name: ReleaseDirectApplication :exec
-- Gives up a delivery claim after the email failed, so the candidate's retry is immediate
-- rather than waiting out the claim.
UPDATE direct_applications
SET sending_at = NULL
WHERE id = $1 AND sent_at IS NULL;

-- name: GetDirectApplication :one
-- One candidate's application to one vacancy, if any — whether it reached the employer is
-- sent_at, whether a delivery is in flight is a sending_at younger than the claim's two
-- minutes. Read by the apply form ("already applied") and by Apply to tell the three
-- reasons UpsertDirectApplication can return nothing apart.
SELECT id, sent_at,
       (sending_at IS NOT NULL AND sending_at >= now() - interval '2 minutes')::boolean AS sending
FROM direct_applications
WHERE job_id = $1 AND user_id = $2;

-- The employer's inbox. Every statement below reaches an application only through the
-- vacancy it was sent to, and only when that vacancy is the employer's own on-site posting
-- (created_by = employer AND source = 'employer') — the same ownership predicate the
-- employer's vacancy writes use (jobs_manual.sql). An application not yet delivered
-- (sent_at NULL) is not the employer's to see: they were never sent it, and it may still be
-- replaced by the candidate's retry. The vacancy is read from the base table, not jobs_read:
-- these are identity columns, not a posting read.

-- name: ListEmployerApplications :many
-- One page of the employer's delivered applications, newest first, optionally narrowed to
-- one vacancy and one stage — or to the one application `id` names, which is how a single
-- application is read. Reached through jobs_created_by_idx.
SELECT da.id, j.public_slug AS job_slug, j.title AS job_title, u.email AS candidate_email,
       da.cv_kind, da.note, da.answers, da.stage, da.stage_changed_at, da.sent_at,
       EXISTS (SELECT 1 FROM direct_application_cvs c WHERE c.application_id = da.id) AS has_cv
FROM direct_applications da
JOIN jobs j ON j.id = da.job_id
JOIN users u ON u.id = da.user_id
WHERE j.created_by = sqlc.arg(employer_id)::bigint
  AND j.source = 'employer'
  AND da.sent_at IS NOT NULL
  AND (sqlc.narg(job_slug)::text IS NULL OR j.public_slug = sqlc.narg(job_slug)::text)
  AND (sqlc.narg(stage)::text IS NULL OR da.stage = sqlc.narg(stage)::text)
  AND (sqlc.narg(id)::bigint IS NULL OR da.id = sqlc.narg(id)::bigint)
ORDER BY da.sent_at DESC, da.id DESC
LIMIT sqlc.arg(row_limit)::int OFFSET sqlc.arg(row_offset)::int;

-- name: CountEmployerApplicationsByStage :many
-- How many delivered applications sit at each stage, under the same vacancy narrowing as
-- ListEmployerApplications but never its stage one: the inbox shows every stage's count
-- beside the stage it is filtered to. A stage with no application is absent.
SELECT da.stage, count(*)::bigint AS applications
FROM direct_applications da
JOIN jobs j ON j.id = da.job_id
WHERE j.created_by = sqlc.arg(employer_id)::bigint
  AND j.source = 'employer'
  AND da.sent_at IS NOT NULL
  AND (sqlc.narg(job_slug)::text IS NULL OR j.public_slug = sqlc.narg(job_slug)::text)
GROUP BY da.stage;

-- name: GetEmployerApplicationCV :one
-- The CV file a delivered application sent, as the employer received it. No row for an
-- application that is not the employer's, and for one written before the copy was kept.
SELECT c.filename, c.content_type, c.content
FROM direct_application_cvs c
JOIN direct_applications da ON da.id = c.application_id
JOIN jobs j ON j.id = da.job_id
WHERE c.application_id = sqlc.arg(id)
  AND j.created_by = sqlc.arg(employer_id)::bigint
  AND j.source = 'employer'
  AND da.sent_at IS NOT NULL;

-- name: SetDirectApplicationStage :one
-- Moves a delivered application to a stage in the employer's pipeline and reports what the
-- move needs downstream: whether it changed anything (`changed` is false when it already
-- stood there, so a repeated click notifies nobody twice), and who the candidate is and
-- what they applied to. The row is locked first, so of two concurrent moves to the same
-- stage only one reports a change. No row: not the employer's.
WITH target AS (
    SELECT da.id, da.user_id, da.job_id
    FROM direct_applications da
    JOIN jobs j ON j.id = da.job_id
    WHERE da.id = sqlc.arg(id)
      AND j.created_by = sqlc.arg(employer_id)::bigint
      AND j.source = 'employer'
      AND da.sent_at IS NOT NULL
    FOR UPDATE OF da
), moved AS (
    UPDATE direct_applications
    SET stage = sqlc.arg(stage)::text, stage_changed_at = now()
    WHERE id = (SELECT target.id FROM target)
      AND stage <> sqlc.arg(stage)::text
    RETURNING id
)
SELECT EXISTS (SELECT 1 FROM moved) AS changed,
       t.user_id AS candidate_id, u.email AS candidate_email,
       t.job_id, j.public_slug AS job_slug, j.title AS job_title, j.company
FROM target t
JOIN jobs j ON j.id = t.job_id
JOIN users u ON u.id = t.user_id;
