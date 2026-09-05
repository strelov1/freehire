-- name: ListSavedSearches :many
-- A user's saved searches, most recently updated first (the "My filters" picker order).
SELECT * FROM saved_searches
WHERE user_id = $1
ORDER BY updated_at DESC;

-- name: CountSavedSearches :one
-- How many saved searches a user has — the per-user cap is enforced against this in
-- the service before a create.
SELECT count(*) FROM saved_searches
WHERE user_id = $1;

-- name: CreateSavedSearch :one
-- Create a saved search for a user. The UNIQUE (user_id, name) constraint rejects a
-- duplicate name; the partial UNIQUE (user_id) WHERE derived_from_profile rejects a
-- second profile-derived row for the same user (both surfaced by the repository via
-- pgerr.UniqueViolationConstraint, mapped to distinct sentinels). Returns the row.
INSERT INTO saved_searches (user_id, name, query, derived_from_profile)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateSavedSearch :one
-- Overwrite a saved search's name and/or query, scoped to its owner, bumping
-- updated_at. Partial update: a NULL param leaves that column unchanged (COALESCE),
-- so the caller can rename, overwrite the filters, or both in one call. An empty
-- query string is a real value (not NULL), so "save the unfiltered view" is honored.
-- No matching owner-scoped row returns no row (the handler maps that to 404).
UPDATE saved_searches
SET name       = COALESCE(sqlc.narg('name'), name),
    query      = COALESCE(sqlc.narg('query'), query),
    updated_at = now()
WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id')
RETURNING *;

-- name: DeleteSavedSearch :execrows
-- Delete a saved search, scoped to its owner so a user can only delete their own.
-- Returns the affected row count: 0 means it does not exist or is not the caller's
-- (the handler maps that to 404).
DELETE FROM saved_searches
WHERE id = $1 AND user_id = $2;

-- name: GetSavedSearch :one
-- Fetch one of a user's saved searches, owner-scoped. Used by
-- internal/engage/subscription to read the stored query when subscribing. No
-- matching row (wrong id or another user's) returns no row.
SELECT * FROM saved_searches
WHERE id = $1 AND user_id = $2;
