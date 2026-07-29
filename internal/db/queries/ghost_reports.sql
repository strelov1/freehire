-- name: CreateGhostReport :one
-- File one person's claim that they applied to a posting and were never answered.
--
-- Two of the three refusals are STRUCTURAL rather than checks the service performs:
-- the INSERT selects from users and jobs, so an unproven address or a posting already
-- taken down inserts no row at all. That is the same shape CreateAPIKey uses for the
-- verified-address gate — a guarantee nothing downstream can forget to apply.
--
-- A conflicting row that has been RETRACTED is revived rather than refused: retracting
-- by mistake must not lock somebody out of their own claim forever, and reviving cannot
-- inflate anything, since the row (and so the person) is still counted once. A
-- conflicting row that is still live updates nothing (the DO UPDATE's WHERE), returning
-- no row, which the repository reports as a duplicate.
--
-- Because all four outcomes are "no row", the repository asks GhostReportRefusalReason
-- which one it was. That costs an extra query only on the failure path.
INSERT INTO ghost_reports (user_id, job_id, applied_on)
SELECT u.id, j.id, sqlc.arg(applied_on)::date
FROM users u, jobs j
WHERE u.id = sqlc.arg(user_id)::bigint
  AND u.email_verified
  AND j.id = sqlc.arg(job_id)::bigint
  AND j.closed_at IS NULL
ON CONFLICT (user_id, job_id) DO UPDATE
  SET retracted_at = NULL,
      applied_on   = EXCLUDED.applied_on,
      created_at   = now()
  WHERE ghost_reports.retracted_at IS NOT NULL
RETURNING *;

-- name: GhostReportRefusalReason :one
-- Why CreateGhostReport returned no row. Read only on the failure path, so the happy
-- path stays one statement. Each column answers one gate, and the repository maps the
-- first failing one — unverified before closed before duplicate — because an
-- unverified account should be told to confirm its address rather than that somebody
-- already reported the job.
SELECT
  coalesce((SELECT u.email_verified FROM users u
             WHERE u.id = sqlc.arg(user_id)::bigint), false)::boolean AS email_verified,
  coalesce((SELECT j.closed_at IS NULL FROM jobs j
             WHERE j.id = sqlc.arg(job_id)::bigint), false)::boolean AS job_open,
  (EXISTS (SELECT 1 FROM ghost_reports r
            WHERE r.user_id = sqlc.arg(user_id)::bigint
              AND r.job_id = sqlc.arg(job_id)::bigint
              AND r.retracted_at IS NULL))::boolean AS already_reported;

-- name: RetractGhostReport :one
-- Withdraw a live claim. Scoped to a non-retracted row so a second retraction affects
-- nothing and surfaces as not-found, rather than silently re-stamping the date.
UPDATE ghost_reports
SET retracted_at = now()
WHERE user_id = sqlc.arg(user_id)::bigint
  AND job_id = sqlc.arg(job_id)::bigint
  AND retracted_at IS NULL
RETURNING *;

-- name: CountGhostReportsSince :one
-- How many claims this account has filed since a cutoff, for the daily cap. Counts
-- retracted rows too: filing and withdrawing in a loop is exactly the pattern the cap
-- exists to bound, so forgiving it would leave the cap trivially bypassable.
SELECT count(*) FROM ghost_reports
WHERE user_id = sqlc.arg(user_id)::bigint
  AND created_at >= sqlc.arg(since)::timestamptz;
