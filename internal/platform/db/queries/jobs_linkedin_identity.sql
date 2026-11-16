-- name: FindLinkedInJobByURL :one
-- LinkedIn posting IDs are global, while legacy rows were keyed by search keyword.
-- Reuse a public row's existing key and stable identity across any discovery slice.
-- Closed rows remain eligible so ordinary upsert rediscovery policy can reopen them.
-- The normalized URL expression uses jobs_normalized_url_idx without a source scan.
SELECT * FROM jobs_read
WHERE source = 'linkedin' AND NOT is_private
  AND normalize_job_url(url) = normalize_job_url(sqlc.arg(url)::text)
  AND normalize_job_url(sqlc.arg(url)::text) <> ''
ORDER BY (duplicate_of IS NOT NULL), id
LIMIT 1;
