-- name: UpsertDirectApplication :one
-- Records a candidate's application to an on-site vacancy and CLAIMS it for delivery
-- (sending_at), or replaces the content of one they recorded earlier that never reached the
-- employer — the retry after a failed delivery. See migration 0187.
--
-- Returns no row in exactly three cases, which GetDirectApplication tells apart afterwards:
-- the application was already SENT; another request holds a live claim on it (a delivery in
-- flight — claiming it again would email the employer twice); or the candidate has reached
-- today's cap. The cap counts applications created since midnight UTC
-- on OTHER vacancies, and a retry on this one always passes, so a failed delivery never
-- costs a second slot. Two concurrent first applications can both pass the count and land
-- one over the cap; the cap bounds abuse, it is not a quota anyone is billed against.
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
RETURNING id;

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
