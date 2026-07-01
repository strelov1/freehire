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
-- duplicate name (surfaced by the repository as a duplicate-name error). Returns the row.
INSERT INTO saved_searches (user_id, name, query)
VALUES ($1, $2, $3)
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
-- Fetch one of a user's saved searches, owner-scoped. Used by the share use case to
-- read the current name/public_slug before deciding whether to keep an existing slug
-- or mint a new one. No matching row (wrong id or another user's) returns no row (the
-- service maps that to ErrNotFound).
SELECT * FROM saved_searches
WHERE id = $1 AND user_id = $2;

-- name: SetSavedSearchPublicSlug :one
-- Publish a saved search as a board: set its public slug and (optional) author label,
-- owner-scoped, bumping updated_at. The service decides the slug (keeping an existing
-- one on re-share, minting a fresh one otherwise), so this sets it verbatim; a
-- collision with another board's slug raises a UNIQUE violation the service retries.
-- author_label is set verbatim (NULL clears it → anonymous). No matching owner-scoped
-- row returns no row (→ ErrNotFound).
UPDATE saved_searches
SET public_slug  = sqlc.arg('public_slug'),
    author_label = sqlc.narg('author_label'),
    updated_at   = now()
WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id')
RETURNING *;

-- name: ClearSavedSearchPublicSlug :execrows
-- Unpublish a board: clear the slug and author label, owner-scoped. Returns the
-- affected row count: 1 for an owned row (whether or not it was shared — unshare is an
-- idempotent no-op when already private), 0 when missing or not the caller's (→ 404).
UPDATE saved_searches
SET public_slug = NULL, author_label = NULL, updated_at = now()
WHERE id = $1 AND user_id = $2;

-- name: GetPublicBoardBySlug :one
-- Public read of a shared board by its slug — no auth, no owner-scoping. Exposes only
-- the board's display fields; owner columns (user_id) are never selected. A NULL slug
-- never equals the param, so private sets are unreachable. No row → 404.
SELECT name, query, author_label
FROM saved_searches
WHERE public_slug = $1;
