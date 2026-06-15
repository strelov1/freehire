-- name: RecordJobView :one
-- Record (or refresh) a user's view of a job. Idempotent on (user_id, job_id):
-- the first view creates the row, a repeat view touches viewed_at. Returns the
-- row so the caller learns the current applied_at in the same round-trip.
INSERT INTO user_jobs (user_id, job_id)
VALUES ($1, $2)
ON CONFLICT (user_id, job_id) DO UPDATE SET viewed_at = now()
RETURNING *;

-- name: MarkJobApplied :one
-- Mark a job as applied for a user. Idempotent and independent of a prior view:
-- it inserts the row (viewed_at defaults) or updates applied_at in place, and
-- seeds stage='applied' only when the stage is unset (an advanced stage survives
-- a re-apply, via COALESCE).
INSERT INTO user_jobs (user_id, job_id, applied_at, stage)
VALUES ($1, $2, now(), 'applied')
ON CONFLICT (user_id, job_id) DO UPDATE
  SET applied_at = now(), stage = COALESCE(user_jobs.stage, 'applied')
RETURNING *;

-- name: SaveJob :one
-- Save (bookmark) a job for a user. Idempotent and independent of a prior view:
-- it inserts the row (viewed_at defaults) or refreshes saved_at in place.
INSERT INTO user_jobs (user_id, job_id, saved_at)
VALUES ($1, $2, now())
ON CONFLICT (user_id, job_id) DO UPDATE SET saved_at = now()
RETURNING *;

-- name: UnsaveJob :one
-- Clear a job's saved mark without deleting the interaction row, so view and
-- apply history survive unsaving. No interaction row -> pgx.ErrNoRows; the
-- handler treats that as "already not saved", never as a failure.
UPDATE user_jobs
SET saved_at = NULL
WHERE user_id = $1 AND job_id = $2
RETURNING *;

-- name: TrackJob :one
-- Set an application's stage and/or notes for a user, idempotently. Upserts the
-- (user, job) row (viewed_at defaults). Partial update: a NULL param leaves that
-- column unchanged (COALESCE keeps the existing value), so the caller can set the
-- stage, the notes, or both in one call. Returns the row.
INSERT INTO user_jobs (user_id, job_id, stage, notes)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, job_id) DO UPDATE
  SET stage = COALESCE(EXCLUDED.stage, user_jobs.stage),
      notes = COALESCE(EXCLUDED.notes, user_jobs.notes)
RETURNING *;

-- name: ClearJobProgress :one
-- Reset a tracked job to the wishlist: drop stage and applied state, keep saved/viewed/notes.
UPDATE user_jobs
SET stage = NULL, applied_at = NULL
WHERE user_id = $1 AND job_id = $2
RETURNING *;

-- name: UntrackJob :one
-- Remove a job from the board: drop every pipeline mark, keep viewed_at so the
-- job remains in the user's view history.
UPDATE user_jobs
SET saved_at = NULL, applied_at = NULL, stage = NULL, notes = NULL
WHERE user_id = $1 AND job_id = $2
RETURNING *;

-- name: ListUserJobs :many
-- A user's job interactions joined with the job rows, most recently touched
-- first (GREATEST ignores NULLs; viewed_at is always set). filter narrows to
-- viewed-only/saved/applied subsets; 'all' is every interaction, 'viewed' is
-- the passive history (rows neither saved nor applied). Closed jobs stay
-- listed: a user's history must not shrink when a posting closes.
SELECT sqlc.embed(jobs), uj.viewed_at, uj.saved_at, uj.applied_at, uj.stage, uj.notes
FROM user_jobs uj
JOIN jobs ON jobs.id = uj.job_id
WHERE uj.user_id = $1
  AND (sqlc.arg(filter)::text = 'all'
       OR (sqlc.arg(filter)::text = 'viewed' AND uj.saved_at IS NULL AND uj.applied_at IS NULL)
       OR (sqlc.arg(filter)::text = 'saved' AND uj.saved_at IS NOT NULL)
       OR (sqlc.arg(filter)::text = 'applied' AND uj.applied_at IS NOT NULL)
       OR (sqlc.arg(filter)::text = 'board'
           AND (uj.saved_at IS NOT NULL OR uj.applied_at IS NOT NULL OR uj.stage IS NOT NULL)))
ORDER BY GREATEST(uj.viewed_at, uj.saved_at, uj.applied_at) DESC, uj.job_id DESC
LIMIT $2 OFFSET $3;

-- name: ListViewedJobSlugs :many
-- Every public_slug the user has interacted with (viewed_at is always set, so
-- any interaction row counts as viewed). Used by the SPA to dim already-seen
-- cards in the browse list without authenticating the public job-read path.
-- Closed jobs are included: dimming a closed posting that still shows in a
-- history surface is correct, and the browse list filters closed jobs itself.
SELECT jobs.public_slug
FROM user_jobs uj
JOIN jobs ON jobs.id = uj.job_id
WHERE uj.user_id = $1;

-- name: CountUserJobs :one
-- Per-filter row counts for the my-jobs tabs, in one aggregate pass. "all" is
-- every interaction row; "viewed" is the view-only subset (neither saved nor
-- applied), matching the ListUserJobs filter. "board" counts jobs on the Kanban
-- board (saved, applied, or stage set), matching the ListUserJobs board filter.
SELECT count(*)                                        AS "all",
       count(*) FILTER (WHERE saved_at IS NULL
                          AND applied_at IS NULL)      AS viewed,
       count(*) FILTER (WHERE saved_at   IS NOT NULL) AS saved,
       count(*) FILTER (WHERE applied_at IS NOT NULL) AS applied,
       count(*) FILTER (WHERE saved_at   IS NOT NULL
                            OR applied_at IS NOT NULL
                            OR stage      IS NOT NULL) AS board
FROM user_jobs
WHERE user_id = $1;
