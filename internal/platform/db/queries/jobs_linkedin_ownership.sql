-- name: ListLinkedInOwnershipCandidates :many
-- Operator reconciliation reads the existing public discoveries, independently
-- of LinkedIn's bounded recent listing. Only source-pending withdrawals are revisited; genuine closed/private history is untouched.
SELECT * FROM jobs_read
WHERE source = 'linkedin' AND (closed_at IS NULL OR closed_reason = 'native_source_pending')
  AND NOT is_private AND duplicate_of IS NULL
  AND id > sqlc.arg(after_id)::bigint
ORDER BY id
LIMIT sqlc.arg(row_limit);
