-- name: ListJobChanges :many
-- One keyset page of the public change feed, oldest first, after the (xid, id) position
-- the caller holds. The walk is job_changes_xid_id_idx; each entry is joined to its job by
-- primary key, so the cost of a page is its limit, never the size of the log or of jobs.
--
-- Only entries whose transaction is finished are served: xid below the oldest in-flight
-- transaction (pg_snapshot_xmin). An entry can therefore never commit behind a cursor a
-- client already holds. Ordering by id alone would not give that, because ids are taken
-- inside the trigger, long before a bulk close commits.
--
-- The visibility is the search index's: canonical (duplicate_of IS NULL) and public. Not
-- closed_at IS NULL, because a closed entry is exactly what this feed carries. Judged now,
-- not at the time of the change.
SELECT c.id, c.xid, c.kind, c.reason, c.at, j.public_slug
FROM job_changes c
JOIN jobs j ON j.id = c.job_id
WHERE (c.xid, c.id) > (sqlc.arg(after_xid)::bigint, sqlc.arg(after_id)::bigint)
  AND c.xid < pg_snapshot_xmin(pg_current_snapshot())::text::bigint
  AND j.duplicate_of IS NULL AND NOT j.is_private
ORDER BY c.xid, c.id
LIMIT sqlc.arg(row_limit);

-- name: JobChangesStartXID :one
-- Where a `since` begins: the transaction of the earliest entry recorded at or after it,
-- found by job_changes_at_idx. No row (pgx.ErrNoRows) means nothing has changed since.
SELECT xid
FROM job_changes
WHERE at >= sqlc.arg(since)
ORDER BY at, id
LIMIT 1;

-- name: JobChangesAvailableSince :one
-- When the log begins (job_changes_at_idx). NULL while it is empty.
SELECT min(at)::timestamptz AS available_since
FROM job_changes;
