-- name: GetUserJobAnalysis :one
-- The caller's cached fit analysis for one job, with the staleness stamps it was
-- computed against. No row means the pair was never analyzed (the handler serves a
-- null analysis, no LLM call). The handler compares cv_uploaded_at / job_content_hash
-- to the live CV upload time and job content_hash to decide the stale flag.
SELECT analysis, model, cv_uploaded_at, job_content_hash, created_at
FROM user_job_analysis
WHERE user_id = $1 AND job_id = $2;

-- name: UpsertUserJobAnalysis :exec
-- Create-or-replace the cached analysis for a (user, job). The composite PRIMARY KEY
-- makes it idempotent: a recompute overwrites the analysis, model, and both staleness
-- stamps and re-bumps created_at. analysis is the sanitized jobfit.Analysis JSON.
INSERT INTO user_job_analysis (user_id, job_id, analysis, model, cv_uploaded_at, job_content_hash, created_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (user_id, job_id) DO UPDATE
SET analysis         = EXCLUDED.analysis,
    model            = EXCLUDED.model,
    cv_uploaded_at   = EXCLUDED.cv_uploaded_at,
    job_content_hash = EXCLUDED.job_content_hash,
    created_at       = now();
