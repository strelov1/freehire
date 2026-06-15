-- name: CreateReport :one
-- File a user complaint about a job into the moderation queue as 'pending'. The partial
-- unique index on (reported_by, job_id) WHERE status='pending' rejects a second open report
-- of the same job by the same user (the repository maps that unique violation to a 409).
INSERT INTO job_reports (
    reported_by, job_id, reason, details, contact_telegram
) VALUES (
    sqlc.arg(reported_by)::bigint, sqlc.arg(job_id)::bigint, sqlc.arg(reason),
    sqlc.arg(details), sqlc.arg(contact_telegram)
)
RETURNING *;

-- name: GetReport :one
-- Load a single report by id for the review path. The resolve/dismiss flow guards the
-- status in the service; the Mark* queries are additionally scoped to status='pending' as
-- defense-in-depth against a concurrent second decision.
SELECT * FROM job_reports WHERE id = $1;

-- name: ListPendingReports :many
-- The moderator review queue: every pending report, newest first, with the reporter's email
-- and the reported job's slug and title so the moderator can judge it and link to it.
SELECT r.*, u.email AS reporter_email, j.public_slug AS job_slug, j.title AS job_title
FROM job_reports r
JOIN users u ON u.id = r.reported_by
JOIN jobs j ON j.id = r.job_id
WHERE r.status = 'pending'
ORDER BY r.created_at DESC;

-- name: MarkReportResolved :one
-- Mark a pending report resolved, recording the deciding moderator. Scoped to
-- status='pending' so a concurrent second decision affects no row (the service maps 0 rows
-- to ErrAlreadyDecided). The optional job close is a separate write (CloseJobByID).
UPDATE job_reports
SET status      = 'resolved',
    reviewed_by = sqlc.arg(reviewed_by)::bigint,
    reviewed_at = now()
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: MarkReportDismissed :one
-- Mark a pending report dismissed with an optional reason, recording the deciding
-- moderator. Scoped to status='pending' (see MarkReportResolved). The job is not touched.
UPDATE job_reports
SET status        = 'dismissed',
    reviewed_by   = sqlc.arg(reviewed_by)::bigint,
    reviewed_at   = now(),
    review_reason = sqlc.arg(review_reason)
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;
