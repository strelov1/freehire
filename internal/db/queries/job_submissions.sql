-- name: CreateSubmission :one
-- Insert a user-contributed vacancy into the moderation queue as 'pending'. The partial
-- unique index on lower(url) WHERE status='pending' rejects a second pending submission of
-- the same URL (the repository maps that unique violation to a 409).
INSERT INTO job_submissions (
    submitted_by, url, source, title, company, location, remote, description, posted_at
) VALUES (
    sqlc.arg(submitted_by)::bigint, sqlc.arg(url), sqlc.arg(source), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(location), sqlc.arg(remote), sqlc.arg(description),
    sqlc.arg(posted_at)
)
RETURNING *;

-- name: GetSubmission :one
-- Load a single submission by id for the review path. The approve/reject flow guards the
-- status in the service; the Mark* queries are additionally scoped to status='pending' as
-- defense-in-depth against a concurrent second decision.
SELECT * FROM job_submissions WHERE id = $1;

-- name: ListPendingSubmissions :many
-- The moderator review queue: every pending submission, newest first, with the submitter's
-- email so the moderator can judge provenance.
SELECT s.*, u.email AS submitter_email
FROM job_submissions s
JOIN users u ON u.id = s.submitted_by
WHERE s.status = 'pending'
ORDER BY s.created_at DESC;

-- name: ListSubmissionsByUser :many
-- "My submissions": one user's submissions, newest first, whatever their status.
-- LEFT JOIN the minted job (present only once approved) to surface its public_slug,
-- so the UI can link an approved submission straight to its live vacancy page.
SELECT s.*, j.public_slug AS job_slug
FROM job_submissions s
LEFT JOIN jobs j ON j.id = s.job_id
WHERE s.submitted_by = $1
ORDER BY s.created_at DESC;

-- name: MarkSubmissionApproved :one
-- Mark a pending submission approved, recording the deciding moderator and the minted job.
-- Scoped to status='pending' so a concurrent second decision affects no row (the service
-- maps 0 rows to ErrAlreadyDecided). The job is minted by the service before this runs.
UPDATE job_submissions
SET status      = 'approved',
    reviewed_by = sqlc.arg(reviewed_by)::bigint,
    reviewed_at = now(),
    job_id      = sqlc.arg(job_id)::bigint
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: MarkSubmissionRejected :one
-- Mark a pending submission rejected with an optional reason, recording the deciding
-- moderator. Scoped to status='pending' (see MarkSubmissionApproved). No job is created.
UPDATE job_submissions
SET status        = 'rejected',
    reviewed_by   = sqlc.arg(reviewed_by)::bigint,
    reviewed_at   = now(),
    review_reason = sqlc.arg(review_reason)
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;
