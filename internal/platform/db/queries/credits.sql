-- name: ListJobLabelsByIDs :many
-- Resolve job ids to display labels for the credit-history page (match debits). Missing ids
-- simply do not come back; the handler falls back to a generic label for a deleted job.
SELECT id, title, public_slug
FROM jobs
WHERE id = ANY(sqlc.arg(ids)::bigint[]);
