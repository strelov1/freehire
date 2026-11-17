-- name: LockLinkedInPostingIdentity :exec
-- Hold through the caller's transaction, before identity lookup or any posting lock.
-- Concurrent inserts can otherwise fail the public-slug unique check before the
-- (source, external_id) ON CONFLICT branch can select their shared posting.
SELECT pg_advisory_xact_lock(hashtextextended('linkedin_posting:' || sqlc.arg(posting_id)::text, 0));

-- name: FindLinkedInJobByURL :one
-- LinkedIn posting IDs are global, while legacy rows were keyed by search keyword.
-- Reuse a public row's existing key and stable identity across any discovery slice.
-- Closed rows remain eligible so ordinary upsert rediscovery policy can reopen them.
-- The LinkedIn-specific normalized URL index also covers closed public history.
SELECT * FROM jobs_read
WHERE source = 'linkedin' AND NOT is_private
  AND normalize_job_url(url) = normalize_job_url(sqlc.arg(url)::text)
  AND normalize_job_url(sqlc.arg(url)::text) <> ''
ORDER BY (duplicate_of IS NOT NULL), id
LIMIT 1;
