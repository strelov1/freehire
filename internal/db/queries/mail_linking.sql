-- name: GetUserApplication :one
-- The caller's interaction row for one job (the application-detail header).
SELECT viewed_at, saved_at, applied_at, stage, notes
FROM user_jobs
WHERE user_id = $1 AND job_id = $2;

-- name: ListJobEmails :many
-- The emails linked to one of the caller's applications, newest first, for the
-- application detail page.
SELECT id, source, from_addr, from_name, subject, status_signal, link_source,
    received_at, (read_at IS NOT NULL)::boolean AS read
FROM emails
WHERE user_id = $1 AND job_id = $2
ORDER BY received_at DESC, id DESC;

-- name: ConfirmEmailLink :execrows
-- Promote a suggested link to a confirmed one: the suggestion becomes job_id with
-- link_source 'manual'. No-op (0 rows) when there is no pending suggestion.
UPDATE emails
SET job_id           = suggested_job_id,
    link_source      = 'manual',
    suggested_job_id = NULL
WHERE id = $1 AND user_id = $2 AND suggested_job_id IS NOT NULL;

-- name: RejectEmailLink :execrows
-- Dismiss a suggestion without linking.
UPDATE emails
SET suggested_job_id = NULL
WHERE id = $1 AND user_id = $2 AND suggested_job_id IS NOT NULL;

-- name: LinkEmailToJob :execrows
-- Manually link (or relink) an email to a chosen application, overriding any
-- auto-link or suggestion.
UPDATE emails
SET job_id           = $3,
    link_source      = 'manual',
    suggested_job_id = NULL
WHERE id = $1 AND user_id = $2;

-- name: UnlinkEmail :execrows
-- Clear an email's application link (leaves the classified status intact).
UPDATE emails
SET job_id      = NULL,
    link_source = NULL
WHERE id = $1 AND user_id = $2;