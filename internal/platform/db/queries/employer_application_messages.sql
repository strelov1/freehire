-- name: GetEmployerApplicationMessageTarget :one
SELECT u.email_verified,
       EXISTS (SELECT 1 FROM public.direct_application_messages m
               WHERE m.application_id = da.id AND m.status = 'received' AND m.read_at IS NULL) AS has_unread_reply
FROM public.direct_applications da
JOIN public.jobs j ON j.id = da.job_id
JOIN public.users u ON u.id = da.user_id
WHERE da.id = sqlc.arg(application_id)::bigint AND da.sent_at IS NOT NULL
  AND j.source = 'employer' AND NOT j.is_private
  AND j.created_by IN (SELECT public.active_employer_team(sqlc.arg(employer_id)::bigint));

-- name: ListEmployerApplicationMessages :many
SELECT m.id, m.body, m.created_at, m.request_key,
       (CASE WHEN m.status = 'sending' AND m.updated_at <= now() - interval '2 minutes'
             THEN 'failed' ELSE m.status END)::text AS status
FROM public.direct_application_messages m
JOIN public.direct_applications da ON da.id = m.application_id
JOIN public.jobs j ON j.id = da.job_id
WHERE m.application_id = sqlc.arg(application_id)::bigint AND da.sent_at IS NOT NULL
  AND j.source = 'employer' AND NOT j.is_private
  AND j.created_by IN (SELECT public.active_employer_team(sqlc.arg(employer_id)::bigint))
  AND (sqlc.arg(before_id)::bigint = 0 OR m.id < sqlc.arg(before_id)::bigint)
ORDER BY m.id DESC
LIMIT sqlc.arg(row_limit)::int;

-- name: BeginEmployerApplicationMessage :one
-- One caller can send for a request key. A different body or destination cannot reuse it.
WITH target AS (
    SELECT da.id, u.email, j.title, j.company
    FROM public.direct_applications da
    JOIN public.jobs j ON j.id = da.job_id
    JOIN public.users u ON u.id = da.user_id
    WHERE da.id = sqlc.arg(application_id)::bigint AND da.sent_at IS NOT NULL
      AND j.source = 'employer' AND NOT j.is_private AND u.email_verified
      AND j.created_by IN (SELECT public.active_employer_team(sqlc.arg(employer_id)::bigint))
), started AS (
    INSERT INTO public.direct_application_messages AS m
        (application_id, body, status, request_key, reply_token, recipient_email)
    SELECT target.id, sqlc.arg(body)::text, 'sending', sqlc.arg(request_key)::uuid, gen_random_uuid(), target.email
    FROM target
    ON CONFLICT (application_id, request_key) WHERE request_key IS NOT NULL DO UPDATE
    SET status = 'sending', updated_at = now()
    WHERE m.body = EXCLUDED.body AND m.recipient_email = EXCLUDED.recipient_email
      AND (m.status = 'failed' OR (m.status = 'sending' AND m.updated_at <= now() - interval '2 minutes'))
    RETURNING m.id, m.reply_token, m.updated_at
)
SELECT started.id, started.reply_token, started.updated_at, target.email, target.title, target.company
FROM started CROSS JOIN target;

-- name: FinishEmployerApplicationMessage :exec
UPDATE public.direct_application_messages
SET status = sqlc.arg(status)::text, updated_at = now()
WHERE id = sqlc.arg(id)::bigint AND status = 'sending' AND updated_at = sqlc.arg(started_at)::timestamptz;

-- name: GetEmployerApplicationMessageRequest :one
SELECT m.id, m.body, m.created_at, m.request_key, m.status
FROM public.direct_application_messages m
JOIN public.direct_applications da ON da.id = m.application_id
JOIN public.jobs j ON j.id = da.job_id
JOIN public.users u ON u.id = da.user_id
WHERE m.application_id = sqlc.arg(application_id)::bigint AND da.sent_at IS NOT NULL
  AND m.request_key = sqlc.arg(request_key)::uuid AND m.body = sqlc.arg(body)::text
  AND m.recipient_email = u.email AND u.email_verified
  AND j.source = 'employer' AND NOT j.is_private
  AND j.created_by IN (SELECT public.active_employer_team(sqlc.arg(employer_id)::bigint));

-- name: MarkEmployerApplicationRepliesRead :exec
UPDATE public.direct_application_messages m
SET read_at = now()
FROM public.direct_applications da, public.jobs j
WHERE m.application_id = da.id AND da.job_id = j.id
  AND da.id = sqlc.arg(application_id)::bigint AND da.sent_at IS NOT NULL
  AND j.source = 'employer' AND NOT j.is_private
  AND j.created_by IN (SELECT public.active_employer_team(sqlc.arg(employer_id)::bigint))
  AND m.status = 'received' AND m.read_at IS NULL AND m.id <= sqlc.arg(through_id)::bigint;

-- name: InsertEmployerApplicationReply :exec
-- Called only for an authenticated SES sender and its envelope recipient.
INSERT INTO public.direct_application_messages (application_id, body, status, external_id)
SELECT m.application_id, sqlc.arg(body)::text, 'received', sqlc.arg(external_id)::text
FROM public.direct_application_messages m
JOIN public.direct_applications da ON da.id = m.application_id
JOIN public.users u ON u.id = da.user_id
WHERE m.reply_token = sqlc.arg(reply_token)::uuid AND da.sent_at IS NOT NULL
  AND u.email_verified AND lower(u.email) = lower(sqlc.arg(sender)::text)
  AND lower(m.recipient_email) = lower(u.email)
ON CONFLICT (application_id, external_id) WHERE external_id IS NOT NULL DO NOTHING;
