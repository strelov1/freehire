-- name: ClaimProblemReportWrite :one
INSERT INTO problem_report_daily (user_id, day, writes)
VALUES ($1, $2, 1)
ON CONFLICT (user_id, day) DO UPDATE
SET writes = problem_report_daily.writes + 1
WHERE problem_report_daily.writes < 20
RETURNING writes;

-- name: RecordProblemReport :one
INSERT INTO problem_reports (user_id, category, goal, expected, actual, tool_name, error_code, fingerprint, request_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (user_id, fingerprint) WHERE status = 'open'
DO UPDATE SET request_count = problem_reports.request_count + 1,
              last_reported_at = now(), request_id = EXCLUDED.request_id
RETURNING id, status, request_count, first_reported_at, last_reported_at;
