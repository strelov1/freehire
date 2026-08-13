-- name: GetScreeningAnswers :one
-- The caller's single screening-answers record, keyed by user_id. No matching row means
-- the candidate has not stated any screening answer yet.
SELECT * FROM screening_answers
WHERE user_id = $1;

-- name: UpsertScreeningAnswers :one
-- Create-or-replace the caller's one screening-answers record. Full-replace, mirroring
-- UpsertUserProfile: the service reads the current row, merges caller-provided fields over
-- it (omitted fields keep their stored value), and writes the merged result back whole —
-- so the SQL layer stays a plain upsert and the partial-update semantics live in Go, where
-- they are unit-testable without a database.
INSERT INTO screening_answers (
    user_id, authorized_countries, visa_sponsorship_needed,
    desired_salary_amount, desired_salary_currency, desired_salary_period,
    notice_period_days, willing_to_relocate, age_18_or_older
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (user_id) DO UPDATE
SET authorized_countries    = EXCLUDED.authorized_countries,
    visa_sponsorship_needed = EXCLUDED.visa_sponsorship_needed,
    desired_salary_amount   = EXCLUDED.desired_salary_amount,
    desired_salary_currency = EXCLUDED.desired_salary_currency,
    desired_salary_period   = EXCLUDED.desired_salary_period,
    notice_period_days      = EXCLUDED.notice_period_days,
    willing_to_relocate     = EXCLUDED.willing_to_relocate,
    age_18_or_older         = EXCLUDED.age_18_or_older,
    updated_at               = now()
RETURNING *;
