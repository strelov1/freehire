-- name: UpsertDirectApplication :one
-- Records a candidate's application to an on-site vacancy, or replaces the content of one
-- they recorded earlier that never reached the employer (sent_at NULL) — the retry after a
-- failed delivery. See migration 0186.
--
-- Returns no row in exactly two cases, which GetDirectApplication tells apart afterwards:
-- the application was already SENT (the conflict's WHERE refuses to touch it), or the
-- candidate has reached today's cap. The cap counts applications created since midnight UTC
-- on OTHER vacancies, and a retry on this one always passes, so a failed delivery never
-- costs a second slot. Two concurrent first applications can both pass the count and land
-- one over the cap; the cap bounds abuse, it is not a quota anyone is billed against.
INSERT INTO direct_applications (job_id, user_id, cv_id, note, answers)
SELECT @job_id, @user_id, @cv_id, @note, @answers
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
SET cv_id = EXCLUDED.cv_id, note = EXCLUDED.note, answers = EXCLUDED.answers
WHERE direct_applications.sent_at IS NULL
RETURNING id;

-- name: MarkDirectApplicationSent :exec
-- Stamps an application delivered. Only ever after the email succeeded.
UPDATE direct_applications
SET sent_at = now()
WHERE id = $1 AND sent_at IS NULL;

-- name: GetDirectApplication :one
-- One candidate's application to one vacancy, if any — whether it reached the employer is
-- sent_at. Read by the apply form ("already applied") and by Apply to tell a sent
-- application from a reached cap after UpsertDirectApplication returned nothing.
SELECT id, sent_at
FROM direct_applications
WHERE job_id = $1 AND user_id = $2;
