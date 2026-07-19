-- name: RecordJobView :one
-- Record (or refresh) a user's view of a job. Idempotent on (user_id, job_id):
-- the first view creates the row, a repeat view touches viewed_at. Returns the
-- row so the caller learns the current applied_at in the same round-trip.
-- When (and only when) the row is created for the first time — no prior
-- interaction existed — bump the job's materialized view_count in the same
-- statement. All WITH sub-statements see one snapshot, so `prior` reflects the
-- pre-upsert state regardless of execution order; a repeat view never re-bumps.
WITH prior AS (
    SELECT 1 AS existed FROM user_jobs uj WHERE uj.user_id = $1 AND uj.job_id = $2
), upsert AS (
    INSERT INTO user_jobs (user_id, job_id)
    VALUES ($1, $2)
    ON CONFLICT (user_id, job_id) DO UPDATE SET viewed_at = now()
    RETURNING *
), bump AS (
    UPDATE jobs SET view_count = view_count + 1
    WHERE id = $2 AND NOT EXISTS (SELECT 1 FROM prior)
)
SELECT * FROM upsert;

-- name: MarkJobApplied :one
-- Mark a job as applied for a user. Idempotent and independent of a prior view:
-- it inserts the row (viewed_at defaults) or updates applied_at in place, and
-- seeds stage='applied' only when the stage is unset (an advanced stage survives
-- a re-apply, via COALESCE). When (and only when) applied_at transitions from
-- unset to set, bump the job's materialized applied_count in the same statement;
-- `prior` sees the pre-upsert applied_at, so a re-apply never re-bumps.
WITH prior AS (
    SELECT uj.applied_at FROM user_jobs uj WHERE uj.user_id = $1 AND uj.job_id = $2
), upsert AS (
    INSERT INTO user_jobs (user_id, job_id, applied_at, stage)
    VALUES ($1, $2, now(), 'applied')
    ON CONFLICT (user_id, job_id) DO UPDATE
      SET applied_at = now(), stage = COALESCE(user_jobs.stage, 'applied')
    RETURNING *
), bump AS (
    UPDATE jobs SET applied_count = applied_count + 1
    WHERE id = $2 AND NOT EXISTS (SELECT 1 FROM prior WHERE prior.applied_at IS NOT NULL)
)
SELECT * FROM upsert;

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

-- name: DismissJob :one
-- Dismiss (swipe away) a job for a user in the swipe deck. Idempotent and
-- independent of a prior view: it inserts the row (viewed_at defaults) or
-- refreshes dismissed_at in place.
INSERT INTO user_jobs (user_id, job_id, dismissed_at)
VALUES ($1, $2, now())
ON CONFLICT (user_id, job_id) DO UPDATE SET dismissed_at = now()
RETURNING *;

-- name: UndismissJob :one
-- Clear a job's dismissed mark without deleting the interaction row, so view/
-- apply/save history survives. No interaction row -> pgx.ErrNoRows; the handler
-- treats that as "already not dismissed", never as a failure. This is the undo
-- path for a swipe-left decision.
UPDATE user_jobs
SET dismissed_at = NULL
WHERE user_id = $1 AND job_id = $2
RETURNING *;

-- name: ExcludedJobIDs :many
-- Job ids the user has already interacted with (viewed, saved, applied, or
-- dismissed) — the swipe deck's exclusion set, so a card is shown at most once
-- across sessions. viewed_at is set on every interaction row, so any row for the
-- user counts (the deck records a view the moment a card is shown). Ordered
-- most-recently-touched first and capped ($2) so the deck's `id NOT IN (...)`
-- search filter stays bounded; the overflow risk is only an occasional re-shown
-- long-ago-seen job, never a correctness problem.
SELECT job_id
FROM user_jobs
WHERE user_id = $1
ORDER BY GREATEST(viewed_at, saved_at, applied_at, dismissed_at) DESC
LIMIT $2;

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
-- A user's job interactions joined with the job rows. Each subset is ordered by
-- when the job entered *that* list, not by last touch: saved by saved_at, applied
-- by applied_at, the passive history by viewed_at, the board by when it was saved
-- or applied. This keeps a plain re-view from bumping a saved/applied job to the
-- top (viewed_at is refreshed on every view). 'all' keeps the touched-recency
-- timeline. filter narrows to viewed-only/saved/applied subsets; 'viewed' is the
-- passive history (rows neither saved nor applied). Closed jobs stay listed: a
-- user's history must not shrink when a posting closes. email_count is the
-- caller's live (non-deleted) inbox messages linked to this job — the board's
-- per-card ✉ badge; 0 for everyone without a connected mailbox. reminder_fire_at is
-- the pending saved-job reminder's deadline (NULL when none), so the saved list can
-- show "remind in N days" with its reschedule/off controls.
SELECT sqlc.embed(jobs), uj.viewed_at, uj.saved_at, uj.applied_at, uj.stage, uj.notes,
       (SELECT count(*)
          FROM emails e
         WHERE e.user_id = uj.user_id
           AND e.job_id = jobs.id
           AND e.deleted_at IS NULL) AS email_count,
       (SELECT r.fire_at
          FROM job_reminders r
         WHERE r.user_id = uj.user_id
           AND r.job_id = jobs.id
           AND r.status = 'pending') AS reminder_fire_at
FROM user_jobs uj
JOIN jobs ON jobs.id = uj.job_id
WHERE uj.user_id = $1
  AND (sqlc.arg(filter)::text = 'all'
       OR (sqlc.arg(filter)::text = 'viewed' AND uj.saved_at IS NULL AND uj.applied_at IS NULL)
       OR (sqlc.arg(filter)::text = 'saved' AND uj.saved_at IS NOT NULL)
       OR (sqlc.arg(filter)::text = 'applied' AND uj.applied_at IS NOT NULL)
       OR (sqlc.arg(filter)::text = 'board'
           AND (uj.saved_at IS NOT NULL OR uj.applied_at IS NOT NULL OR uj.stage IS NOT NULL)))
ORDER BY (CASE sqlc.arg(filter)::text
            WHEN 'saved' THEN uj.saved_at
            WHEN 'applied' THEN uj.applied_at
            WHEN 'viewed' THEN uj.viewed_at
            WHEN 'board' THEN GREATEST(uj.saved_at, uj.applied_at)
            ELSE GREATEST(uj.viewed_at, uj.saved_at, uj.applied_at)
          END) DESC NULLS LAST, uj.job_id DESC
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

-- name: ListSavedJobSlugs :many
-- Every public_slug the user has saved (bookmarked). Used by the SPA to render
-- the save toggle as filled on already-saved cards in the browse list and search
-- results, without authenticating the public job-read path — the saved set is
-- cross-referenced client-side, never joined into ListJobs/SearchJobs. Bounded by
-- the caller's saved subset (typically small) and indexed by the (user_id, job_id)
-- primary key, so it stays cheap for heavy users. Closed jobs are included: a
-- saved posting that later closes still shows filled in a history surface.
SELECT jobs.public_slug
FROM user_jobs uj
JOIN jobs ON jobs.id = uj.job_id
WHERE uj.user_id = $1 AND uj.saved_at IS NOT NULL;

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

-- name: CountMyJobsByStage :many
-- Per-stage application counts for the Pipeline snapshot. An application is any
-- row the user applied to or staged (saved-only rows are excluded); a row with
-- applied_at set but no stage groups under a NULL stage. The Go layer folds these
-- rows into the pipeline buckets.
SELECT stage, count(*) AS count
FROM user_jobs
WHERE user_id = $1
  AND (applied_at IS NOT NULL OR stage IS NOT NULL)
GROUP BY stage;
