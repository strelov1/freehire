-- name: CreateCV :one
-- Insert a new CV for a user. data is the sanitized structured document (JSON). This is the
-- user's own CV, not a copy made for a vacancy, so is_tailored is stated false rather than left to
-- be inferred from a NULL job_id — an absence deleting the vacancy also produces. Users may own several of
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
SELECT c.id, c.title, c.template_id, c.data, c.job_id, c.is_tailored, c.agent_session_id,
       c.autopilot_report, c.tracer_links_enabled, c.base_cv_id, c.hand_edited,
       -- The company this copy was tailored for, which is what a tracer token is prefixed with:
       -- the recruiter reads it on hover, and their own company's name alarms less than an
       -- opaque string. Empty for a base CV, and for a tailored copy whose vacancy was deleted.
       coalesce(j.company_slug, '') AS company_slug,
       c.created_at, c.updated_at
FROM cvs c
LEFT JOIN jobs j ON j.id = c.job_id
WHERE c.id = $1 AND c.user_id = $2;

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
-- The user's DEFAULT base CV — the one used wherever a single base is needed without asking
-- (a tailor request that names none, auto-apply, reseed, the profile and header readers). Used
-- as the seed source when tailoring; returns no row when the user has only tailored CVs or none
-- at all (the caller then seeds a base from the extracted résumé), INCLUDING when their only
-- vacancy-less CV is a tailored copy whose vacancy was deleted. Excluding on is_tailored rather
-- than requiring `job_id IS NULL` is the whole point: the absence of a vacancy link is something
-- deleting a vacancy creates, so it cannot mean "not tailored".
--
-- The rule: (1) the base the user's most recently created tailored CV was made from, while that
-- base still exists; else (2) the OLDEST created base. The newest copy counts even when its
-- base_cv_id has gone NULL (its base was deleted): that is "no live base", which falls to (2), not
-- a reason to look at an older copy. Not "most recently edited": bases are
-- edited by hand now, and a default that flipped with every typo fix would make "which base does
-- tailoring use" unanswerable. ListBaseCVsByUser computes the same expression for its is_default
-- flag — change both together.
SELECT b.id, b.title, b.template_id, b.data, b.hand_edited, b.created_at, b.updated_at
FROM cvs b
WHERE b.user_id = $1 AND NOT b.is_tailored
ORDER BY (b.id = (
            SELECT t.base_cv_id FROM cvs t
            WHERE t.user_id = $1 AND t.is_tailored
            ORDER BY t.created_at DESC, t.id DESC
            LIMIT 1
          )) DESC NULLS LAST,
         b.created_at ASC, b.id ASC
LIMIT 1;

-- name: ListBaseCVsByUser :many
-- Every base (non-tailored) CV of a user as metadata, with is_default marking the one
-- GetBaseCVByUser would return. Oldest created first, so the list does not reorder when a base
-- is edited.
SELECT b.id, b.title, b.template_id, b.hand_edited, b.created_at, b.updated_at,
       (b.id = (
          SELECT d.id FROM cvs d
          WHERE d.user_id = $1 AND NOT d.is_tailored
          ORDER BY (d.id = (
                      SELECT t.base_cv_id FROM cvs t
                      WHERE t.user_id = $1 AND t.is_tailored
                      ORDER BY t.created_at DESC, t.id DESC
                      LIMIT 1
                    )) DESC NULLS LAST,
                   d.created_at ASC, d.id ASC
          LIMIT 1
        ))::boolean AS is_default
FROM cvs b
WHERE b.user_id = $1 AND NOT b.is_tailored
ORDER BY b.created_at ASC, b.id ASC;

-- name: DuplicateBaseCV :one
-- Copy an owned BASE CV: same stored document, template and hand_edited flag, a new title.
-- Copies the stored (already sanitized) document server-side so it never round-trips through a
-- client. Owner-scoped and base-only: a foreign, missing or tailored id returns no row (the
-- caller tells those apart with a prior read).
INSERT INTO cvs (user_id, title, template_id, data, is_tailored, hand_edited)
SELECT src.user_id, $3, src.template_id, src.data, false, src.hand_edited
FROM cvs src
WHERE src.id = $1 AND src.user_id = $2 AND NOT src.is_tailored
RETURNING id, title, template_id, created_at, updated_at;

-- name: MarkCVHandEdited :execrows
-- Record that the candidate changed this CV through the editor. Only ever sets the flag.
UPDATE cvs
SET hand_edited = true
WHERE id = $1 AND user_id = $2 AND NOT hand_edited;

-- name: CreateTailoredCV :one
-- Insert a CV bound to a vacancy (job_id set) — the per-vacancy tailored copy. data is the
-- sanitized document copied from the base CV. is_tailored is stated, not inferred: if this vacancy
-- is ever deleted the row loses its job_id, and the flag is what keeps it a tailored copy instead of
-- joining the pool the base is chosen from. Returns the metadata the detail response needs.
INSERT INTO cvs (user_id, title, template_id, data, job_id, is_tailored, base_cv_id)
VALUES ($1, $2, $3, $4, $5, true, sqlc.narg(base_cv_id))
RETURNING id, title, template_id, created_at, updated_at;

-- name: SetCVAutopilotReport :execrows
-- Replace the run report on an owned CV. The agent writes the WHOLE report on every call —
-- there is no partial update, so a requirement closed later from the candidate's own words
-- arrives as the same list with one entry changed. Owner-scoped: 0 rows for a foreign id.
UPDATE cvs
SET autopilot_report = $3
WHERE id = $1 AND user_id = $2;

-- name: MergeCVAutopilotEntry :execrows
-- Fold ONE requirement's outcome into the run report: replace the entry whose requirement
-- matches case- and whitespace-insensitively, or append when none does. Done as one UPDATE
-- expression rather than a read-modify-write from Go, because a read-modify-write has no lock
-- between its two calls — two concurrent merges (a duplicate tool call, two requests racing
-- during a run) can both read the same starting report and each overwrite the other's entry.
-- The CASE here evaluates against one row snapshot under Postgres's own row-level lock for the
-- UPDATE, so nothing else can observe or write the row mid-merge. Owner-scoped: 0 rows for a
-- foreign id.
UPDATE cvs
SET autopilot_report = CASE
    WHEN EXISTS (
      SELECT 1 FROM jsonb_array_elements(COALESCE(autopilot_report, '[]'::jsonb)) elem
      WHERE lower(trim(elem ->> 'requirement')) = lower(trim(sqlc.arg(entry)::jsonb ->> 'requirement'))
    )
    THEN (
      SELECT jsonb_agg(
        CASE WHEN lower(trim(elem ->> 'requirement')) = lower(trim(sqlc.arg(entry)::jsonb ->> 'requirement'))
             THEN sqlc.arg(entry)::jsonb ELSE elem END
      )
      FROM jsonb_array_elements(COALESCE(autopilot_report, '[]'::jsonb)) elem
    )
    ELSE COALESCE(autopilot_report, '[]'::jsonb) || jsonb_build_array(sqlc.arg(entry)::jsonb)
  END
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
