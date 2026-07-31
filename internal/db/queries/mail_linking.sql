-- name: GetUserApplication :one
-- The caller's interaction row for one job (the application-detail header).
-- last_activity_at and has_pending_suggestion mirror ListUserJobs deliberately: the follow-up gate
-- must reach the same silence verdict as the badge on the board, and two derivations of one rule
-- drift. See the column comment in 0059 for why followed_up_at is NOT part of the activity.
SELECT uj.viewed_at, uj.saved_at, a.applied_at, a.stage, a.notes, a.followed_up_at,
       (CASE WHEN a.applied_at IS NOT NULL THEN
          GREATEST(a.applied_at,
                   (SELECT max(e.received_at)
                      FROM emails e
                     WHERE e.user_id = uj.user_id
                       AND e.job_id = uj.job_id
                       AND e.deleted_at IS NULL))
        END)::timestamptz AS last_activity_at,
       (a.applied_at IS NOT NULL AND EXISTS (
          SELECT 1
            FROM emails e
           WHERE e.user_id = uj.user_id
             AND e.suggested_job_id = uj.job_id
             -- "Pending" means the caller has not confirmed it, and confirming is
             -- what attaches the application — the same test the inbox's link
             -- filter makes, so the two cannot disagree about one message.
             AND e.application_id IS NULL
             AND e.deleted_at IS NULL))::boolean AS has_pending_suggestion
FROM user_jobs uj
LEFT JOIN applications a ON a.user_id = uj.user_id AND a.job_id = uj.job_id
WHERE uj.user_id = $1 AND uj.job_id = $2;

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
    application_id   = (SELECT a.id FROM applications a
                         WHERE a.user_id = emails.user_id AND a.job_id = emails.suggested_job_id),
    link_source      = 'manual',
    suggested_job_id = NULL
WHERE emails.id = $1 AND emails.user_id = $2 AND emails.suggested_job_id IS NOT NULL;

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
    application_id   = (SELECT a.id FROM applications a
                         WHERE a.user_id = emails.user_id AND a.job_id = $3),
    link_source      = 'manual',
    suggested_job_id = NULL
WHERE emails.id = $1 AND emails.user_id = $2;

-- name: UnlinkEmail :execrows
-- Clear an email's application link (leaves the classified status intact).
UPDATE emails
SET job_id         = NULL,
    application_id = NULL,
    link_source = NULL
WHERE id = $1 AND user_id = $2;