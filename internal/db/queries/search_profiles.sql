-- name: ListSearchProfiles :many
-- A user's search profiles, most recently updated first (the profile picker order).
SELECT * FROM search_profiles
WHERE user_id = $1
ORDER BY updated_at DESC;

-- name: CountSearchProfiles :one
-- How many profiles a user has — the per-user cap is enforced against this in the
-- service before a create.
SELECT count(*) FROM search_profiles
WHERE user_id = $1;

-- name: CreateSearchProfile :one
-- Create a profile for a user. The UNIQUE (user_id, name) constraint rejects a
-- duplicate name (surfaced by the repository as a duplicate-name error). Specializations
-- and skills are already normalized by the service. Returns the row.
INSERT INTO search_profiles (user_id, name, specializations, skills)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateSearchProfile :one
-- Overwrite a profile's name, specializations, and/or skills, scoped to its owner,
-- bumping updated_at. Partial update: a NULL param leaves that column unchanged
-- (COALESCE), so the caller can rename, re-specialize, replace skills, or any
-- combination in one call. No matching owner-scoped row returns no row (the handler
-- maps that to 404).
UPDATE search_profiles
SET name            = COALESCE(sqlc.narg('name'), name),
    specializations = COALESCE(sqlc.narg('specializations'), specializations),
    skills          = COALESCE(sqlc.narg('skills'), skills),
    updated_at      = now()
WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id')
RETURNING *;

-- name: GetSearchProfile :one
-- One profile scoped to its owner, so a user can only read their own. No matching row
-- (wrong id or another user's) returns no row (the handler maps that to 404).
SELECT * FROM search_profiles
WHERE id = $1 AND user_id = $2;

-- name: DeleteSearchProfile :execrows
-- Delete a profile, scoped to its owner so a user can only delete their own. Returns
-- the affected row count: 0 means it does not exist or is not the caller's (the
-- handler maps that to 404).
DELETE FROM search_profiles
WHERE id = $1 AND user_id = $2;
