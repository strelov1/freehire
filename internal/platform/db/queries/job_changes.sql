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
-- Visibility was judged when the change was logged (migration 0263: canonical, public, open
-- all feed one "listed" flag), so it is not re-judged here, with one privacy guard: a job
-- that is private NOW serves only `closed` entries. Those carry just a slug the client was
-- already given while it was public, and withholding them would leave the copy listed.
SELECT c.id, c.xid, c.kind, c.reason, c.at, j.public_slug
FROM job_changes c
JOIN jobs j ON j.id = c.job_id
WHERE (c.xid, c.id) > (sqlc.arg(after_xid)::bigint, sqlc.arg(after_id)::bigint)
  AND c.xid < LEAST(
        pg_snapshot_xmin(pg_current_snapshot())::text::bigint,
        -- The settle delay, as a prefix of the key order so the cursor stays safe: nothing at
        -- or after the earliest entry younger than the delay is served, and an entry older
        -- than it but behind a younger one waits (see migration 0263, "Settle rule").
        COALESCE((SELECT y.xid FROM job_changes y WHERE y.at >= now() - sqlc.arg(settle)::interval ORDER BY y.at, y.id LIMIT 1),
                 9223372036854775807))
  AND (NOT j.is_private OR c.kind = 'closed')
ORDER BY c.xid, c.id
LIMIT sqlc.arg(row_limit);

-- name: JobChangesStartXID :one
-- Where a `since` begins: the LOWEST transaction id among the entries recorded at or after it
-- (job_changes_at_idx range). The earliest entry's xid is not enough: a transaction that took
-- its xid first and logged later has a lower xid and would be skipped. The start can reach a
-- little before `since` (a boundary entry may repeat), which a client absorbs through seq.
-- -1 when nothing was recorded since.
SELECT COALESCE(min(xid), -1)::bigint AS xid
FROM job_changes
WHERE at >= sqlc.arg(since);

-- name: JobChangesAvailableSince :one
-- When the log begins (job_changes_at_idx). NULL while it is empty.
SELECT min(at)::timestamptz AS available_since
FROM job_changes;

-- name: PruneJobChanges :execrows
-- Retention for the feed, one bounded batch (the caller loops): entries older than the
-- cutoff, by job_changes_at_idx. See jobChangesRetention in cmd/rollup-stats.
DELETE FROM job_changes d
WHERE d.id IN (
    SELECT o.id FROM job_changes o WHERE o.at < sqlc.arg(cutoff) ORDER BY o.at LIMIT sqlc.arg(batch_size)
);
