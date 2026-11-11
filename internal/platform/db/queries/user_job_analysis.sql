-- name: GetUserJobAnalysis :one
-- The caller's cached fit analysis for one job, with the four staleness stamps it was
-- computed against: model, cv_uploaded_at, job_content_hash, language. No row means the
-- pair was never analyzed (the handler serves a null analysis, no LLM call). The handler
-- compares all four against the live model, CV upload time, job content_hash and profile
-- language to decide the stale flag.
SELECT analysis, model, cv_uploaded_at, job_content_hash, language, created_at
FROM user_job_analysis
WHERE user_id = $1 AND job_id = $2;

-- name: UpsertUserJobAnalysis :exec
-- Create-or-replace the cached analysis for a (user, job). The composite PRIMARY KEY
-- makes it idempotent: a recompute overwrites the analysis and all four staleness stamps
-- (model, cv_uploaded_at, job_content_hash, language). created_at is deliberately NOT
-- re-bumped on conflict, so the tracking list keeps the first-analysis time even
-- when the caller recomputes the same job.
-- analysis is the sanitized matchanalysis.Analysis JSON.
INSERT INTO user_job_analysis (user_id, job_id, analysis, model, cv_uploaded_at, job_content_hash, language, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (user_id, job_id) DO UPDATE
SET analysis         = EXCLUDED.analysis,
    model            = EXCLUDED.model,
    cv_uploaded_at   = EXCLUDED.cv_uploaded_at,
    job_content_hash = EXCLUDED.job_content_hash,
    language         = EXCLUDED.language;

-- name: ListUserJobAnalyses :many
-- Jobs the caller has analyzed, newest first, joined to the job for display. Powers
-- the Tracking → AI fit tab. Includes closed jobs (surfaced with a badge). The four
-- stored staleness stamps (model, cv_uploaded_at, job_content_hash, language) ride
-- along so the handler can flag rows whose CV, job, model, or profile language has
-- since changed, and the analysis blob carries the overall score + verdict the list
-- shows. Capped at 500 because each row carries a full analysis JSONB over the wire.
SELECT j.public_slug, j.title, j.company, j.closed_at, j.content_hash,
       a.analysis, a.model, a.cv_uploaded_at, a.job_content_hash, a.language, a.created_at
FROM user_job_analysis a
JOIN jobs j ON j.id = a.job_id
WHERE a.user_id = $1
ORDER BY a.created_at DESC
LIMIT 500;
