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
-- Load a single report by id for the review path, with the reporter's email and the
-- reported job's slug and title — the decision notice needs them, and joining here spares
-- the decision path a second round trip. The resolve/dismiss flow guards the status in the
-- service; the Mark* queries are additionally scoped to status='pending' as defense-in-depth
-- against a concurrent second decision.
SELECT r.*, u.email AS reporter_email, j.public_slug AS job_slug, j.title AS job_title
FROM job_reports r
JOIN users u ON u.id = r.reported_by
JOIN jobs j ON j.id = r.job_id
WHERE r.id = $1;

-- name: ListPendingReports :many
-- The moderator review queue: pending reports, newest first, with the reporter's email
-- and the reported job's slug and title so the moderator can judge it and link to it.
-- Capped at 500 as a runaway-growth guard — far above any plausible backlog; a queue
-- that deep needs bulk triage, not a longer page.
SELECT r.*, u.email AS reporter_email, j.public_slug AS job_slug, j.title AS job_title
FROM job_reports r
JOIN users u ON u.id = r.reported_by
JOIN jobs j ON j.id = r.job_id
WHERE r.status = 'pending'
ORDER BY r.created_at DESC
LIMIT 500;

-- name: MarkReportResolved :one
-- Mark a pending report resolved, recording the deciding moderator and their note. The note
-- shares review_reason with dismiss: both answer "why the moderator decided this", and both
-- are quoted back to the reporter in the decision notice. Scoped to status='pending' so a
-- concurrent second decision affects no row (the service maps 0 rows to ErrAlreadyDecided).
-- The optional job close is a separate write (CloseJobByID).
UPDATE job_reports
SET status        = 'resolved',
    reviewed_by   = sqlc.arg(reviewed_by)::bigint,
    reviewed_at   = now(),
    review_reason = sqlc.arg(review_reason)
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

-- name: CountReportsFiledSince :one
-- How many reports this account has filed since a cutoff, for the daily cap
-- (ghost_reports.CountGhostReportsSince's counterpart for this queue). Counts every status,
-- not just pending: a report already resolved or dismissed still consumed the reporter's
-- daily allowance, so excluding it would let a decided report be re-filed for free.
SELECT count(*) FROM job_reports
WHERE reported_by = sqlc.arg(reported_by)::bigint
  AND created_at >= sqlc.arg(since)::timestamptz;
