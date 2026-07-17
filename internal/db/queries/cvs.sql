-- name: CreateCV :one
-- Insert a new CV for a user. data is the sanitized structured document (JSON). job_id
-- defaults NULL (the tailoring seam is unused in phase 1). Returns the metadata the list
-- and detail responses need.
INSERT INTO cvs (user_id, title, template_id, data)
VALUES ($1, $2, $3, $4)
RETURNING id, title, template_id, created_at, updated_at;

-- name: ListCVsByUser :many
-- A user's CVs as metadata (no data blob), newest edit first.
SELECT id, title, template_id, created_at, updated_at
FROM cvs
WHERE user_id = $1
ORDER BY updated_at DESC;

-- name: GetCVByID :one
-- One CV owned by the user, including the full data blob. Owner-scoped: a foreign or
-- missing id returns no row (the handler maps it to 404). job_id is NULL for a base CV and
-- the vacancy id for a tailored copy — the tailoring-context read resolves it to the analysis.
SELECT id, title, template_id, data, job_id, created_at, updated_at
FROM cvs
WHERE id = $1 AND user_id = $2;

-- name: UpdateCV :one
-- Replace a CV's editable fields, stamping updated_at. Owner-scoped: no row is updated
-- for a foreign or missing id (the handler maps the resulting no-row error to 404).
UPDATE cvs
SET title = $3, template_id = $4, data = $5, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING id, title, template_id, created_at, updated_at;

-- name: DeleteCV :execrows
-- Delete a CV owned by the user. Returns the affected-row count so the handler can 404
-- when nothing was deleted (foreign or missing id).
DELETE FROM cvs
WHERE id = $1 AND user_id = $2;

-- name: GetBaseCVByUser :one
-- The user's base CV (job_id IS NULL) — their non-tailored résumé, newest edit first. Used
-- as the seed source when tailoring; returns no row when the user has only tailored CVs or
-- none at all (the caller then seeds a base from the extracted résumé).
SELECT id, title, template_id, data, created_at, updated_at
FROM cvs
WHERE user_id = $1 AND job_id IS NULL
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: CreateTailoredCV :one
-- Insert a CV bound to a vacancy (job_id set) — the per-vacancy tailored copy. data is the
-- sanitized document copied from the base CV. Returns the metadata the detail response needs.
INSERT INTO cvs (user_id, title, template_id, data, job_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, title, template_id, created_at, updated_at;
