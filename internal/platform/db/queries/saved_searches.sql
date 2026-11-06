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
--
-- A changed query drops its subscriptions' pending matches in the same statement: they
-- answered the OLD query, and without this the next digest delivers them, including jobs
-- below a minimum skill match the edit just added. DELETE rather than expired_at, because
-- the ledger's unique key would otherwise stop a job the new query also matches from being
-- recorded again. A leased row is left alone, since a send already holds it. last_searched_at
-- stays: clearing it would re-read a page of postings the subscriber was never owed.
--
-- The DELETE only covers rows that exist now. query_revision (raised here, by one, only when
-- the query text differs) is what covers the rest: each match records the revision it
-- answered, so a leased row that is released or whose lease lapses, and a match recorded by
-- a pass that read the old query, are ignored by claiming and refused by recording.
WITH dropped AS (
    DELETE FROM subscription_matches m
    USING subscriptions s, saved_searches ss
    WHERE m.subscription_id = s.id
      AND s.saved_search_id = ss.id
      AND ss.id = sqlc.arg('id')
      AND ss.user_id = sqlc.arg('user_id')
      AND ss.query <> sqlc.narg('query')
      AND m.notified_at IS NULL
      AND m.claimed_at IS NULL
      AND m.failed_at IS NULL
      AND m.expired_at IS NULL
)
UPDATE saved_searches
SET name       = COALESCE(sqlc.narg('name'), name),
    query      = COALESCE(sqlc.narg('query'), query),
    query_revision = query_revision
        + CASE WHEN sqlc.narg('query')::text IS NOT NULL AND sqlc.narg('query')::text <> query THEN 1 ELSE 0 END,
    updated_at = now()
WHERE saved_searches.id = sqlc.arg('id') AND saved_searches.user_id = sqlc.arg('user_id')
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
