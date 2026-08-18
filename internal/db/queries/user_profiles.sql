-- name: GetUserProfile :one
-- The caller's single profile, keyed by user_id. No matching row means the user has not
-- saved a profile yet (the handler maps that to a null payload / 404 on sub-resources).
SELECT * FROM user_profiles
WHERE user_id = $1;

-- name: UpsertUserProfile :one
-- Create-or-replace the user's one profile. The PRIMARY KEY (user_id) makes this an
-- idempotent upsert: first save inserts, later saves overwrite specializations/skills/
-- excluded_skills/location_preferences and bump updated_at. All fields are already
-- normalized by the service; excluded_skills may be empty; location_preferences is a
-- validated JSONB block or NULL (no preferences).
INSERT INTO user_profiles (user_id, specializations, skills, excluded_skills, location_preferences)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id) DO UPDATE
SET specializations      = EXCLUDED.specializations,
    skills               = EXCLUDED.skills,
    excluded_skills      = EXCLUDED.excluded_skills,
    location_preferences = EXCLUDED.location_preferences,
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
    excluded_skills      = $4,
    location_preferences = $5,
    updated_at           = now()
WHERE user_id = $1 AND updated_at = $6
RETURNING *;

-- name: DeleteUserProfile :execrows
-- Remove the caller's profile. Returns the affected row count (0 when none existed); the
-- handler treats delete as idempotent (204 either way).
DELETE FROM user_profiles
WHERE user_id = $1;

-- name: ListUserProfilesExcludedSkills :many
-- excluded_skills for a batch of users, one round trip regardless of batch size. A user_id
-- with no profile row simply produces no row here; the caller treats absence as an empty
-- exclude set.
SELECT user_id, excluded_skills FROM user_profiles
WHERE user_id = ANY(sqlc.arg(user_ids)::bigint[]);
