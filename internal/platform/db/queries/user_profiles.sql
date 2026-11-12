-- name: GetUserProfile :one
-- The caller's single profile, keyed by user_id. No matching row means the user has not
-- saved a profile yet (the handler maps that to a null payload / 404 on sub-resources).
SELECT * FROM user_profiles
WHERE user_id = $1;

-- name: CreateUserProfileIfAbsent :one
-- A partial update based on a missing-row snapshot must not replace a profile that
-- another writer created meanwhile. No returned row means the caller must reload.
INSERT INTO user_profiles (user_id, specializations, skills, seniorities, excluded_skills, excluded_sources, excluded_companies, location_preferences, job_languages)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE(sqlc.arg(job_languages)::text[], '{}'::text[]))
ON CONFLICT (user_id) DO NOTHING
RETURNING *;

-- name: UpsertUserProfile :one
-- Create-or-replace the user's one profile. The PRIMARY KEY (user_id) makes this an
-- idempotent upsert: first save inserts, later saves overwrite specializations/skills/
-- seniorities/excluded_skills/excluded_sources/excluded_companies/location_preferences and
-- bump updated_at. All fields are already normalized by the service; seniorities,
-- excluded_skills, excluded_sources and excluded_companies may be empty;
-- location_preferences is a validated JSONB block or NULL (no preferences).
INSERT INTO user_profiles (user_id, specializations, skills, seniorities, excluded_skills, excluded_sources, excluded_companies, location_preferences, job_languages)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE(sqlc.arg(job_languages)::text[], '{}'::text[]))
ON CONFLICT (user_id) DO UPDATE
SET specializations      = EXCLUDED.specializations,
    skills               = EXCLUDED.skills,
    seniorities          = EXCLUDED.seniorities,
    excluded_skills      = EXCLUDED.excluded_skills,
    excluded_sources     = EXCLUDED.excluded_sources,
    excluded_companies   = EXCLUDED.excluded_companies,
    location_preferences = EXCLUDED.location_preferences,
    job_languages        = EXCLUDED.job_languages,
    updated_at           = now()
RETURNING *;

-- name: UpsertUserProfileIfUnchanged :one
-- Same write as UpsertUserProfile, guarded on the row's updated_at still matching what the
-- caller read. Used by MergeSkills, whose merge (which fields to keep, which skills to add)
-- is computed in Go from a prior Get, outside any transaction: a Save() landing in that gap
-- must not be silently clobbered by a write built from a now-stale snapshot. No matching row
-- (updated_at moved, or the profile was deleted) returns zero rows; the caller re-reads and
-- retries rather than overwriting blind.
UPDATE user_profiles
SET specializations      = $2,
    skills               = $3,
    seniorities          = $4,
    excluded_skills      = $5,
    excluded_sources     = $6,
    excluded_companies   = $7,
    location_preferences = $8,
    job_languages        = $10,
    updated_at           = now()
WHERE user_id = $1 AND updated_at = $9
RETURNING *;

-- name: DeleteUserProfile :execrows
-- Remove the caller's profile. Returns the affected row count (0 when none existed); the
-- handler treats delete as idempotent (204 either way).
DELETE FROM user_profiles
WHERE user_id = $1;

-- name: ListUserProfileSkillSets :many
-- skills and excluded_skills for a batch of users, one round trip regardless of batch size.
-- A user_id with no profile row simply produces no row here; the caller treats absence as
-- empty sets.
SELECT user_id, skills, excluded_skills FROM user_profiles
WHERE user_id = ANY(sqlc.arg(user_ids)::bigint[]);
