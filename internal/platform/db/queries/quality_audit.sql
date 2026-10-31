-- The sampled quality audit (cmd/quality-audit, internal/job/qualityaudit): reads a random
-- sample of the catalogue to check it against the world, and keeps the remote judgements.
-- Nothing here writes to jobs — the audit measures, it never acts.
--
-- Both samples draw the same way: a Bernoulli pre-filter (`random() < fraction`, the caller
-- sizing the fraction to a few times the sample) and then ORDER BY random() over what passed,
-- so the sort is over a few dozen rows instead of a source's whole catalogue, and the result is
-- still uniform — a LIMIT without the sort would favour whatever order the scan visits rows in.

-- name: ListLivenessAuditSources :many
-- Sources whose open public postings reach the floor, from Layer 1's snapshot, so the two
-- layers share their denominator.
SELECT source, open_jobs
FROM source_stats
WHERE open_jobs >= sqlc.arg(min_open)::bigint
ORDER BY source;

-- name: SampleLivenessAuditJobs :many
SELECT id, source, external_id, url, public_slug
FROM jobs
WHERE source = sqlc.arg(source)
  AND closed_at IS NULL
  AND NOT is_private
  AND random() < sqlc.arg(fraction)::float8
ORDER BY random()
LIMIT sqlc.arg(max_rows)::int;

-- name: CountOpenRemoteJobsBySource :many
-- How many open public postings each source tags remote — the weights the week's sources are
-- drawn by. Narrow: no description is read.
SELECT source, count(*)::bigint AS remote_jobs
FROM jobs
WHERE closed_at IS NULL
  AND NOT is_private
  AND work_mode = 'remote'
GROUP BY source
HAVING count(*) >= sqlc.arg(min_remote)::bigint
ORDER BY source;

-- name: SampleRemoteAuditJobs :many
SELECT id, source, public_slug, title, location, description,
       ('global' = ANY(regions))::boolean AS tagged_global
FROM jobs
WHERE source = sqlc.arg(source)
  AND closed_at IS NULL
  AND NOT is_private
  AND work_mode = 'remote'
  AND random() < sqlc.arg(fraction)::float8
ORDER BY random()
LIMIT sqlc.arg(max_rows)::int;

-- name: InsertRemoteAuditJudgement :exec
INSERT INTO remote_audit_judgements (job_id, source, work_mode, worldwide, tagged_global)
VALUES (sqlc.arg(job_id), sqlc.arg(source), sqlc.arg(work_mode), sqlc.arg(worldwide), sqlc.arg(tagged_global));

-- name: TallyRemoteAuditJudgements :many
-- Per source, the judgements since the cutoff — the figures the gauges publish. Raw counts:
-- what share is "accurate enough" is the alert rule's to decide.
SELECT source,
       count(*)::bigint                                                         AS sampled,
       count(*) FILTER (WHERE work_mode = 'remote')::bigint                      AS confirmed,
       count(*) FILTER (WHERE work_mode IN ('hybrid', 'onsite'))::bigint         AS contradicted,
       count(*) FILTER (WHERE tagged_global)::bigint                             AS global_sampled,
       count(*) FILTER (WHERE tagged_global AND worldwide = 'yes')::bigint       AS global_confirmed,
       count(*) FILTER (WHERE tagged_global AND worldwide = 'no')::bigint        AS global_contradicted
FROM remote_audit_judgements
WHERE judged_at >= sqlc.arg(since)::timestamptz
GROUP BY source
ORDER BY source;

-- name: LatestRemoteAuditJudgement :one
-- When the remote check last judged anything, NULL when it never has.
SELECT max(judged_at)::timestamptz AS judged_at
FROM remote_audit_judgements;
