-- name: CreateCV :one
-- Insert a new CV for a user. data is the sanitized structured document (JSON). This is the
-- user's own CV, not a copy made for a vacancy, so is_tailored is stated false rather than left to
-- be inferred from a NULL job_id — an absence cmd/prune also produces. Users may own several of
-- these. Returns the metadata the list and detail responses need.
INSERT INTO cvs (user_id, title, template_id, data, is_tailored)
VALUES ($1, $2, $3, $4, false)
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
-- The autopilot columns are reported as the report itself plus a boolean: the pre-run snapshot
-- is a whole second document, and no caller needs its bytes — only whether one exists.
SELECT id, title, template_id, data, job_id, is_tailored, agent_session_id,
       autopilot_report,
       created_at, updated_at
FROM cvs
WHERE id = $1 AND user_id = $2;

-- name: SetCVSession :execrows
-- Bind (or rebind) the agent session to an owned CV. Owner-scoped: returns 0 affected rows for
-- a foreign or missing id (the handler maps that to 404).
UPDATE cvs
SET agent_session_id = $3
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
-- The user's base CV — their non-tailored résumé. Used as the seed source when tailoring;
-- returns no row when the user has only tailored CVs or none at all (the caller then seeds a base
-- from the extracted résumé), INCLUDING when their only vacancy-less CV is a tailored copy whose
-- vacancy was pruned. Excluding on is_tailored rather than requiring `job_id IS NULL` is the whole
-- point: the absence of a vacancy link is something cmd/prune creates, so it cannot mean "not
-- tailored".
--
-- "The base" stays a derived notion — the most recently edited non-tailored CV. Users may own
-- several (cv-builder), so there is no uniqueness constraint and the ORDER BY does the choosing.
SELECT id, title, template_id, data, created_at, updated_at
FROM cvs
WHERE user_id = $1 AND NOT is_tailored
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: CreateTailoredCV :one
-- Insert a CV bound to a vacancy (job_id set) — the per-vacancy tailored copy. data is the
-- sanitized document copied from the base CV. is_tailored is stated, not inferred: if this vacancy
-- is ever pruned the row loses its job_id, and the flag is what keeps it a tailored copy instead of
-- joining the pool the base is chosen from. Returns the metadata the detail response needs.
INSERT INTO cvs (user_id, title, template_id, data, job_id, is_tailored)
VALUES ($1, $2, $3, $4, $5, true)
RETURNING id, title, template_id, created_at, updated_at;

-- name: SetCVAutopilotReport :execrows
-- Replace the run report on an owned CV. The agent writes the WHOLE report on every call —
-- there is no partial update, so a requirement closed later from the candidate's own words
-- arrives as the same list with one entry changed. Owner-scoped: 0 rows for a foreign id.
UPDATE cvs
SET autopilot_report = $3
WHERE id = $1 AND user_id = $2;

-- name: GetTailoredCVForJob :one
-- The user's existing tailored copy for one vacancy, newest first. The tailoring bootstrap is
-- reached by an address (/tailor/<slug>) that carries no CV reference, so a reload runs the
-- same request again: without this read every refresh minted another copy and stranded the
-- conversation bound to the previous one.
SELECT id, title, template_id, data, agent_session_id, created_at, updated_at
FROM cvs
WHERE user_id = $1 AND job_id = $2
ORDER BY updated_at DESC, id DESC
LIMIT 1;
