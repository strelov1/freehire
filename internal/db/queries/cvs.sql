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

-- name: ListTailoredCVsByUser :many
-- A user's TAILORED CVs (bound to a vacancy), newest edit first — the re-open list. Carries the
-- vacancy's public slug and the bound agent session so each row links back to its workspace.
-- Base CVs (job_id NULL) are excluded; the JOIN also drops tailored CVs whose job was deleted.
SELECT c.id, c.title, c.template_id, c.agent_session_id, j.public_slug AS job_slug,
       j.title AS job_title, j.company AS job_company, c.created_at, c.updated_at
FROM cvs c
JOIN jobs j ON j.id = c.job_id
WHERE c.user_id = $1 AND c.job_id IS NOT NULL
ORDER BY c.updated_at DESC;

-- name: GetCVByID :one
-- One CV owned by the user, including the full data blob. Owner-scoped: a foreign or
-- missing id returns no row (the handler maps it to 404). job_id is NULL for a base CV and
-- the vacancy id for a tailored copy; agent_session_id is the bound roy session (or NULL).
SELECT id, title, template_id, data, job_id, agent_session_id, created_at, updated_at
FROM cvs
WHERE id = $1 AND user_id = $2;

-- name: SetCVSession :execrows
-- Bind (or rebind) the agent session to an owned CV. Owner-scoped: returns 0 affected rows for
-- a foreign or missing id (the handler maps that to 404).
UPDATE cvs
SET agent_session_id = $3
WHERE id = $1 AND user_id = $2;

-- name: SetCVTemplate :execrows
-- Change only a CV's template, stamping updated_at, leaving title and data untouched. Owner-
-- scoped: returns 0 affected rows for a foreign or missing id (the handler maps that to 404).
UPDATE cvs
SET template_id = $3, updated_at = now()
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
