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
-- vacancy it was sent to, and only when that vacancy is an on-site posting of the employer's
-- company team (created_by in employer_team(employer), migration 0271, AND source =
-- 'employer') — the same ownership predicate the employer's vacancy writes use
-- (jobs_manual.sql). An application not yet delivered
-- (sent_at NULL) is not the employer's to see: they were never sent it, and it may still be
-- replaced by the candidate's retry. The vacancy is read from the base table, not jobs_read:
-- these are identity columns, not a posting read.

-- name: ListEmployerApplications :many
-- One page of the employer's delivered applications, newest first, optionally narrowed to
-- one vacancy and one stage — or to the one application `id` names, which is how a single
-- application is read. Reached through jobs_created_by_idx.
SELECT da.id, j.public_slug AS job_slug, j.title AS job_title, u.email AS candidate_email,
       da.cv_kind, da.note, da.answers, da.stage, da.stage_changed_at, da.sent_at, da.viewed_at, da.employer_notes,
       EXISTS (SELECT 1 FROM direct_application_cvs c WHERE c.application_id = da.id) AS has_cv
FROM direct_applications da
JOIN jobs j ON j.id = da.job_id
JOIN users u ON u.id = da.user_id
WHERE j.created_by = ANY(ARRAY(SELECT active_employer_team(sqlc.arg(employer_id)::bigint)))
  AND j.source = 'employer'
  AND da.sent_at IS NOT NULL
  AND (sqlc.narg(job_slug)::text IS NULL OR j.public_slug = sqlc.narg(job_slug)::text)
  AND (sqlc.narg(stage)::text IS NULL OR da.stage = sqlc.narg(stage)::text)
  AND (sqlc.narg(id)::bigint IS NULL OR da.id = sqlc.narg(id)::bigint)
  AND employer_application_matches(COALESCE(sqlc.narg(search)::text, ''), u.email, da.note, da.answers)
ORDER BY da.sent_at DESC, da.id DESC
LIMIT sqlc.arg(row_limit)::int OFFSET sqlc.arg(row_offset)::int;

-- name: CountEmployerApplicationsByStage :many
-- How many delivered applications sit at each stage, under the same vacancy narrowing as
-- ListEmployerApplications but never its stage one: the inbox shows every stage's count
-- beside the stage it is filtered to. A stage with no application is absent.
SELECT da.stage, count(*)::bigint AS applications,
       count(*) FILTER (
           WHERE employer_application_matches(COALESCE(sqlc.narg(search)::text, ''), u.email, da.note, da.answers)
       )::bigint AS matches
FROM direct_applications da
JOIN jobs j ON j.id = da.job_id
JOIN users u ON u.id = da.user_id
WHERE j.created_by = ANY(ARRAY(SELECT active_employer_team(sqlc.arg(employer_id)::bigint)))
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
  AND j.created_by = ANY(ARRAY(SELECT active_employer_team(sqlc.arg(employer_id)::bigint)))
  AND j.source = 'employer'
  AND da.sent_at IS NOT NULL;

-- name: SetDirectApplicationStage :one
-- Stage moves are serialised per application by LockDirectApplication. Capture the
-- previous stage for the audit in the same statement as the write.
WITH target AS MATERIALIZED (
    SELECT da.id, da.stage, j.created_by AS author_id
    FROM direct_applications da
    JOIN jobs j ON j.id = da.job_id
    WHERE da.id = sqlc.arg(id)
      AND j.created_by = ANY(ARRAY(SELECT active_employer_team(sqlc.arg(employer_id)::bigint)))
      AND j.source = 'employer'
      AND da.sent_at IS NOT NULL
    FOR UPDATE OF da
), moved AS (
    UPDATE direct_applications da
    SET stage = sqlc.arg(stage)::text,
        stage_changed_at = CASE WHEN target.stage = sqlc.arg(stage)::text THEN da.stage_changed_at ELSE now() END
    FROM target
    WHERE da.id = target.id
    RETURNING da.id, da.user_id, da.job_id, target.author_id, target.stage AS previous_stage,
              (target.stage <> da.stage)::boolean AS changed
), audited AS (
    INSERT INTO employer_activity (company_slug, actor_id, application_id, action, previous_stage, stage)
    SELECT ca.company_slug, sqlc.arg(employer_id)::bigint, m.id, 'stage_changed', m.previous_stage, sqlc.arg(stage)::text
    FROM moved m
    JOIN company_accounts ca ON ca.user_id = m.author_id OR ca.company_slug = (
        SELECT company_slug FROM company_account_members WHERE user_id = m.author_id
    )
    WHERE m.changed
)
SELECT m.changed, m.user_id AS candidate_id, u.email AS candidate_email,
       m.job_id, j.public_slug AS job_slug, j.title AS job_title, j.company
FROM moved m
JOIN jobs j ON j.id = m.job_id
JOIN users u ON u.id = m.user_id;

-- name: UpdateEmployerApplicationWorkspace :one
WITH target AS MATERIALIZED (
    SELECT da.id, da.employer_notes, j.created_by AS author_id
    FROM direct_applications da
    JOIN jobs j ON j.id = da.job_id
    WHERE da.id = sqlc.arg(id)
      AND j.created_by = ANY(ARRAY(SELECT active_employer_team(sqlc.arg(employer_id)::bigint)))
      AND j.source = 'employer'
      AND da.sent_at IS NOT NULL
    FOR UPDATE OF da
), updated AS (
    UPDATE direct_applications da
    SET employer_notes = COALESCE(sqlc.narg(employer_notes)::text, target.employer_notes),
        viewed_at = CASE WHEN sqlc.arg(mark_viewed)::boolean THEN COALESCE(da.viewed_at, now()) ELSE da.viewed_at END
    FROM target
    WHERE da.id = target.id
    RETURNING da.id, target.author_id, (target.employer_notes <> da.employer_notes) AS notes_changed
), audited AS (
    INSERT INTO employer_activity (company_slug, actor_id, application_id, action)
    SELECT ca.company_slug, sqlc.arg(employer_id)::bigint, updated.id, 'notes_changed'
    FROM updated
    JOIN company_accounts ca ON ca.user_id = updated.author_id OR ca.company_slug = (
        SELECT company_slug FROM company_account_members WHERE user_id = updated.author_id
    )
    WHERE updated.notes_changed
)
SELECT id FROM updated;

-- name: LockDirectApplication :exec
-- Serialises everything one employer move does to one application — the stage write, the
-- candidate's tracking and the letter — against another move of the same application, so
-- an older move can never mirror or mail after a newer one. Session-scoped, because the
-- letter goes out after the move's transaction commits and must still be inside the lock;
-- the caller releases it with UnlockDirectApplication on the same connection.
SELECT pg_advisory_lock(hashtextextended('direct_application:' || sqlc.arg(id)::bigint::text, 0));

-- name: UnlockDirectApplication :one
-- Releases LockDirectApplication; false when this session did not hold it.
SELECT pg_advisory_unlock(hashtextextended('direct_application:' || sqlc.arg(id)::bigint::text, 0))::boolean AS released;
