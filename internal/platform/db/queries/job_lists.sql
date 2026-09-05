-- name: ListJobLists :many
-- A user's job lists, most recently updated first (the account-area order).
SELECT jl.*, count(jli.job_id) AS job_count
FROM job_lists jl
LEFT JOIN job_list_items jli ON jli.list_id = jl.id
WHERE jl.user_id = $1
GROUP BY jl.id
ORDER BY jl.updated_at DESC;

-- name: CountJobLists :one
-- How many lists a user has — the per-user cap is enforced against this in the
-- service before a create.
SELECT count(*) FROM job_lists
WHERE user_id = $1;

-- name: CreateJobList :one
-- Create a job list for a user. The UNIQUE (user_id, name) constraint rejects a
-- duplicate name (surfaced by the repository as a unique-violation). Returns the row.
INSERT INTO job_lists (user_id, name, description)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateJobList :one
-- Overwrite a list's name and/or description, scoped to its owner, bumping
-- updated_at. Partial update: a NULL param leaves that column unchanged (COALESCE).
-- No matching owner-scoped row returns no row (the handler maps that to 404). The
-- job_count subquery matches ListJobLists' so the caller's job_count stays correct
-- after a rename/re-describe, instead of silently reading 0.
UPDATE job_lists
SET name        = COALESCE(sqlc.narg('name'), name),
    description = COALESCE(sqlc.narg('description'), description),
    updated_at  = now()
WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id')
RETURNING *, (SELECT count(*) FROM job_list_items WHERE list_id = job_lists.id) AS job_count;

-- name: DeleteJobList :execrows
-- Delete a list, scoped to its owner. Membership rows cascade; the referenced jobs
-- and the user's separate save flags are untouched. Returns the affected row count:
-- 0 means it does not exist or is not the caller's (the handler maps that to 404).
DELETE FROM job_lists
WHERE id = $1 AND user_id = $2;

-- name: GetJobList :one
-- Fetch one of a user's lists, owner-scoped. Used by share/add/remove to confirm
-- ownership before mutating. No matching row → no row (the service maps that to
-- ErrNotFound).
SELECT * FROM job_lists
WHERE id = $1 AND user_id = $2;

-- name: SetJobListPublicSlug :one
-- Publish a list: set its public slug, owner-scoped, bumping updated_at. The service
-- decides the slug (keeping an existing one on re-share, minting a fresh one
-- otherwise), so this sets it verbatim; a collision with another list's slug raises a
-- UNIQUE violation the service retries. No matching owner-scoped row returns no row
-- (→ ErrNotFound). The job_count subquery matches ListJobLists' so a just-shared
-- list's response carries its real count instead of 0.
UPDATE job_lists
SET public_slug = sqlc.arg('public_slug'),
    updated_at  = now()
WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id')
RETURNING *, (SELECT count(*) FROM job_list_items WHERE list_id = job_lists.id) AS job_count;

-- name: ClearJobListPublicSlug :execrows
-- Unpublish a list: clear its slug, owner-scoped. Returns the affected row count: 1
-- for an owned row (whether or not it was shared — unshare is an idempotent no-op
-- when already private), 0 when missing or not the caller's (→ 404).
UPDATE job_lists
SET public_slug = NULL, updated_at = now()
WHERE id = $1 AND user_id = $2;

-- name: GetPublicJobListBySlug :one
-- Public read of a shared list by its slug — no auth, no owner-scoping. Exposes only
-- the list's display fields; owner columns (user_id) are never selected. A NULL slug
-- never equals the param, so private lists are unreachable. No row → 404.
SELECT id, name, description
FROM job_lists
WHERE public_slug = $1;

-- name: CountJobListItems :one
-- How many jobs a list holds — the per-list cap (maxJobsPerList) is enforced against
-- this in the service before adding a job the list does not already contain. Also
-- what bounds ListJobListItemCards' per-request cost on the public, unauthenticated
-- read: capped write-time membership means a bounded read-time join, never an
-- unbounded one.
SELECT count(*) FROM job_list_items WHERE list_id = $1;

-- name: JobListHasItem :one
-- Whether a job already belongs to a list — lets the service treat re-adding an
-- existing member as free (exempt from the per-list cap) while still capping growth.
SELECT EXISTS (SELECT 1 FROM job_list_items WHERE list_id = $1 AND job_id = $2);

-- name: AddJobListItem :exec
-- Add a job to a list. Idempotent: re-adding an already-present job changes nothing.
INSERT INTO job_list_items (list_id, job_id)
VALUES ($1, $2)
ON CONFLICT (list_id, job_id) DO NOTHING;

-- name: RemoveJobListItem :exec
-- Remove a job from a list. Idempotent: removing an absent job is a no-op.
DELETE FROM job_list_items
WHERE list_id = $1 AND job_id = $2;

-- name: ListJobListMembershipForJob :many
-- A user's job lists, most recently updated first, each flagged with whether the
-- given job is already a member — what the job card's "Add to list" control reads
-- to render its toggle state. jobID is resolved from the job's public slug by the
-- caller before this runs.
SELECT jl.id, jl.name, (jli.job_id IS NOT NULL)::boolean AS in_list
FROM job_lists jl
LEFT JOIN job_list_items jli ON jli.list_id = jl.id AND jli.job_id = sqlc.arg('job_id')
WHERE jl.user_id = sqlc.arg('user_id')
ORDER BY jl.updated_at DESC;

-- name: ListJobListItemCards :many
-- The jobs in a list, newest-added first, projected to a CARD (title, company,
-- status, facets) — never the full row (mirrors ListUserJobs: the description
-- alone would dwarf everything else a card needs). Closed/expired jobs stay
-- listed: a list is the user's own record of what they looked at, not a live
-- availability feed.
SELECT jobs.id, jobs.public_slug, jobs.title, jobs.company, jobs.closed_at,
       jobs.work_mode, jobs.seniority, jobs.employment_type,
       jobs.countries, jobs.regions, jobs.skills, jobs.collections,
       jobs.posted_at, jobs.created_at,
       COALESCE(
           NULLIF(jobs.enrichment ->> 'summary', ''),
           left(regexp_replace(jobs.description, '<[^>]*>', ' ', 'g'), 400)
       )::text AS blurb,
       ARRAY(SELECT jsonb_array_elements_text(jobs.enrichment -> 'countries'))::text[] AS llm_countries,
       ARRAY(SELECT jsonb_array_elements_text(jobs.enrichment -> 'regions'))::text[]   AS llm_regions
FROM job_list_items jli
JOIN jobs ON jobs.id = jli.job_id
WHERE jli.list_id = $1
ORDER BY jli.added_at DESC;

