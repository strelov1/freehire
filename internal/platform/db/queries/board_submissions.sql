-- name: InsertBoardSubmission :one
-- Queue an unclassified URL for triage. The unique index on (url) rejects a duplicate
-- submission of the same link; the caller maps that violation the same way a duplicate
-- board contribution is mapped.
INSERT INTO board_submissions (url, submitted_by, surface)
VALUES (sqlc.arg(url), sqlc.arg(submitted_by), sqlc.arg(surface))
RETURNING *;

-- name: DeleteBoardSubmission :execrows
-- Remove a submission once triage has resolved its (provider, board) and inserted the
-- corresponding boards row.
DELETE FROM board_submissions WHERE id = sqlc.arg(id);

-- name: ListBoardSubmissionsBySubmitter :many
-- One user's still-unclassified submissions, newest first — the other half of the "my
-- contributions" list (boards holds the recognized half).
SELECT * FROM board_submissions
WHERE submitted_by = sqlc.arg(submitted_by)
ORDER BY created_at DESC;
