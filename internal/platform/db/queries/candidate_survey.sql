-- name: GetCandidateSurvey :one
-- The caller's single survey record, keyed by user_id. No matching row means the candidate
-- has answered nothing yet, which is a normal state and not an error — the service turns it
-- into an all-unstated record rather than a 404.
SELECT * FROM candidate_survey
WHERE user_id = $1;

-- name: UpsertCandidateSurvey :one
-- Create-or-replace the caller's one survey record. Full-replace, mirroring
-- UpsertScreeningAnswers: the service reads the current row, merges caller-provided fields
-- over it (an omitted field keeps its stored value; there is deliberately no operation that
-- returns a stated field to unstated — see survey.Merge), and writes the merged result back
-- whole. So the SQL layer stays a plain upsert and the partial-update semantics live in Go,
-- where they are unit-testable without a database.
INSERT INTO candidate_survey (
    user_id, job_search_stage, biggest_challenge, biggest_challenge_note,
    current_income_amount, current_income_currency, current_income_period
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (user_id) DO UPDATE
SET job_search_stage        = EXCLUDED.job_search_stage,
    biggest_challenge       = EXCLUDED.biggest_challenge,
    biggest_challenge_note  = EXCLUDED.biggest_challenge_note,
    current_income_amount   = EXCLUDED.current_income_amount,
    current_income_currency = EXCLUDED.current_income_currency,
    current_income_period   = EXCLUDED.current_income_period,
    updated_at              = now()
RETURNING *;
