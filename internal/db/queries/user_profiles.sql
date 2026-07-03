-- name: GetUserProfile :one
-- The caller's single profile, keyed by user_id. No matching row means the user has not
-- saved a profile yet (the handler maps that to a null payload / 404 on sub-resources).
SELECT * FROM user_profiles
WHERE user_id = $1;

-- name: UpsertUserProfile :one
-- Create-or-replace the user's one profile. The PRIMARY KEY (user_id) makes this an
-- idempotent upsert: first save inserts, later saves overwrite specializations/skills and
-- bump updated_at. Specializations and skills are already normalized by the service.
INSERT INTO user_profiles (user_id, specializations, skills)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE
SET specializations = EXCLUDED.specializations,
    skills          = EXCLUDED.skills,
    updated_at      = now()
RETURNING *;

-- name: DeleteUserProfile :execrows
-- Remove the caller's profile. Returns the affected row count (0 when none existed); the
-- handler treats delete as idempotent (204 either way).
DELETE FROM user_profiles
WHERE user_id = $1;
