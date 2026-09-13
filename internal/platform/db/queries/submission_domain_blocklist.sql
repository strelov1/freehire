-- name: IsHostBlocked :one
-- Whether a normalized host (see submission.normalizeHost) is on the submission blocklist.
SELECT EXISTS (
    SELECT 1 FROM submission_domain_blocklist WHERE host = sqlc.arg(host)
) AS blocked;

-- name: BlockHost :exec
-- Add a host to the blocklist, attributed to the blocking moderator. ON CONFLICT DO NOTHING
-- makes blocking an already-blocked host a no-op rather than an error (submission.Service.Reject
-- calls this every time block_domain is set, whether or not the host is new).
INSERT INTO submission_domain_blocklist (host, blocked_by, reason)
VALUES (sqlc.arg(host), sqlc.arg(blocked_by)::bigint, sqlc.arg(reason))
ON CONFLICT (host) DO NOTHING;
