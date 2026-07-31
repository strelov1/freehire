-- name: RecordJobView :one
-- Record (or refresh) a user's view of a job. Idempotent on (user_id, job_id):
-- the first view creates the row, a repeat view touches viewed_at. Returns the
-- row so the caller learns the current applied_at in the same round-trip. This
-- does NOT touch jobs.view_count — that materialized counter is maintained solely
-- by the nginx-log aggregation worker (cmd/rollup-views), which counts all traffic
-- (anonymous + signed-in + API), so the signed-in beacon must not double-count.
INSERT INTO user_jobs (user_id, job_id)
VALUES ($1, $2)
ON CONFLICT (user_id, job_id) DO UPDATE SET viewed_at = now()
RETURNING *;

-- name: MarkJobApplied :one
-- Mark a job as applied for a user. Idempotent and independent of a prior view:
-- it inserts the row (viewed_at defaults) or updates applied_at in place, and
-- seeds stage='applied' only when the stage is unset (an advanced stage survives
-- a re-apply, via COALESCE). When (and only when) applied_at transitions from
-- unset to set, bump the job's materialized applied_count in the same statement;
-- `prior` sees the pre-upsert applied_at, so a re-apply never re-bumps.
-- MUST run inside a transaction that took LockJobForApply first: `prior` reads
-- the per-statement snapshot, so without the serializing lock two concurrent
-- applies would both see applied_at unset and double-bump (same pattern as
-- LockJobForVote for the vote counters).
-- `at` is the optional recording date, used when an application is reconstructed
-- from employer mail: the application demonstrably existed by the time they
-- wrote, so the mail's timestamp is an honest upper bound and now() would
-- compress its real history. A dated recording also never rewrites an earlier
-- application's date, whereas an ordinary re-apply refreshes it as it always
-- has. Both paths share this one statement so the applied_count transition rule
-- below cannot fork.
-- The `applied` ledger event is written by the same statement, under the same
-- predicate as the counter bump, for the same reason: two records of one
-- transition, decided separately, would eventually disagree. `event_source` is
-- the appevent source of the recording (user, assistant, or a mail source when
-- the application is reconstructed from employer mail); occurred_at is taken
-- from the row's own applied_at, so a dated recording carries the mail's date
-- into the ledger rather than now().
WITH prior AS (
    SELECT uj.applied_at FROM user_jobs uj WHERE uj.user_id = $1 AND uj.job_id = $2
), upsert AS (
    INSERT INTO user_jobs (user_id, job_id, applied_at, stage)
    VALUES ($1, $2, COALESCE(sqlc.narg(at)::timestamptz, now()), 'applied')
    ON CONFLICT (user_id, job_id) DO UPDATE
      SET applied_at = CASE
              WHEN sqlc.narg(at)::timestamptz IS NOT NULL
                  THEN COALESCE(user_jobs.applied_at, sqlc.narg(at)::timestamptz)
              ELSE now()
          END,
          stage = COALESCE(user_jobs.stage, 'applied')
    RETURNING *
), bump AS (
    UPDATE jobs SET applied_count = applied_count + 1
    WHERE id = $2 AND NOT EXISTS (SELECT 1 FROM prior WHERE prior.applied_at IS NOT NULL)
), app AS (
    -- The application record, written by this same statement so it cannot diverge from
    -- the apply it describes. The employer and role title are copied off the posting
    -- here, while it is still there; that copy is what survives cmd/prune.
    --
    -- DO UPDATE rather than DO NOTHING, and not for the update's sake: DO NOTHING
    -- returns no row on conflict, and this statement needs the id in both cases — a
    -- re-apply must name the same application, never a NULL. The assignments mirror the
    -- upsert above: the date refreshes as it always has, an advanced stage survives.
    INSERT INTO applications (user_id, company_slug, role_title, job_id, applied_at, stage)
    SELECT $1, j.company_slug, j.title, $2, u.applied_at, u.stage
      FROM upsert u JOIN jobs j ON j.id = $2
    ON CONFLICT (user_id, job_id) WHERE job_id IS NOT NULL
    DO UPDATE SET applied_at = EXCLUDED.applied_at,
                  stage      = COALESCE(applications.stage, EXCLUDED.stage)
    RETURNING id
), event AS (
    INSERT INTO application_events (user_id, application_id, job_id, company_slug, kind, occurred_at, source)
    SELECT $1, (SELECT id FROM app), $2, j.company_slug, 'applied', u.applied_at, sqlc.arg(event_source)::text
      FROM upsert u JOIN jobs j ON j.id = $2
     WHERE NOT EXISTS (SELECT 1 FROM prior WHERE prior.applied_at IS NOT NULL)
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
-- A `stage_set` ledger event is written when — and only when — the stage actually
-- moves. `prior` reads the pre-upsert value, so re-setting the stage the row
-- already carries, or a notes-only call, records nothing: the ledger holds
-- transitions, and a row per no-op would make "how long did this stage last"
-- unanswerable. The event carries no trusted date (source is the caller, not the
-- employer), which is why nothing times it yet — see internal/appevent.
WITH prior AS (
    SELECT uj.stage FROM user_jobs uj WHERE uj.user_id = $1 AND uj.job_id = $2
), upsert AS (
    INSERT INTO user_jobs (user_id, job_id, stage, notes)
    VALUES ($1, $2, $3, $4)
    ON CONFLICT (user_id, job_id) DO UPDATE
      SET stage = COALESCE(EXCLUDED.stage, user_jobs.stage),
          notes = COALESCE(EXCLUDED.notes, user_jobs.notes)
    RETURNING *
), event AS (
    -- The event names the application, like every other kind does. Resolved through the
    -- posting because that is the link the caller has, and soundly so: this runs at write
    -- time, while the posting is still there. A stage set on a job never applied to has
    -- no application to name and leaves the column NULL — there is nothing to correlate
    -- yet, and inventing a record would assert an application that was never made.
    INSERT INTO application_events (user_id, application_id, job_id, company_slug, kind, signal, occurred_at, source)
    SELECT $1, a.id, $2, j.company_slug, 'stage_set', u.stage, now(), sqlc.arg(event_source)::text
      FROM upsert u
      JOIN jobs j ON j.id = $2
      LEFT JOIN applications a ON a.user_id = $1 AND a.job_id = $2
     WHERE u.stage IS NOT NULL
       AND u.stage IS DISTINCT FROM (SELECT prior.stage FROM prior)
)
SELECT * FROM upsert;

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
           AND r.status = 'pending') AS reminder_fire_at,
       uj.followed_up_at,
       -- last_activity_at is when this application last moved: its apply date, or
       -- the newest message linked to it when that is later. GREATEST ignores a
       -- NULL aggregate, so an application with no mail falls back to applied_at
       -- rather than reporting nothing — a clock that only starts once mail
       -- arrives would never fire on the applications ignored outright, which are
       -- the ones worth reporting. NULL on a row that is not an application: a job
       -- merely viewed or saved is not waiting on anyone.
       (CASE WHEN uj.applied_at IS NOT NULL THEN
          GREATEST(uj.applied_at,
                   (SELECT max(e.received_at)
                      FROM emails e
                     WHERE e.user_id = uj.user_id
                       AND e.job_id = jobs.id
                       AND e.deleted_at IS NULL))
        END)::timestamptz AS last_activity_at,
       -- has_pending_suggestion is mail the matcher believes belongs to this
       -- application that the caller has neither confirmed nor rejected. Such mail
       -- contradicts a claim that nobody replied, so the reader downgrades a
       -- silence verdict to a question rather than asserting it.
       (uj.applied_at IS NOT NULL AND EXISTS (
          SELECT 1
            FROM emails e
           WHERE e.user_id = uj.user_id
             AND e.suggested_job_id = jobs.id
             AND e.job_id IS NULL
             AND e.deleted_at IS NULL))::boolean AS has_pending_suggestion
FROM user_jobs uj
JOIN jobs ON jobs.id = uj.job_id
WHERE uj.user_id = $1
  AND (sqlc.arg(filter)::text = 'all'
       OR (sqlc.arg(filter)::text = 'viewed' AND uj.saved_at IS NULL AND uj.applied_at IS NULL)
       OR (sqlc.arg(filter)::text = 'saved' AND uj.saved_at IS NOT NULL)
       OR (sqlc.arg(filter)::text = 'applied' AND uj.applied_at IS NOT NULL)
       OR (sqlc.arg(filter)::text = 'board'
           AND (uj.saved_at IS NOT NULL OR uj.applied_at IS NOT NULL OR uj.stage IS NOT NULL))
       OR (sqlc.arg(filter)::text = 'dismissed' AND uj.dismissed_at IS NOT NULL))
ORDER BY (CASE sqlc.arg(filter)::text
            WHEN 'saved' THEN uj.saved_at
            WHEN 'applied' THEN uj.applied_at
            WHEN 'viewed' THEN uj.viewed_at
            WHEN 'board' THEN GREATEST(uj.saved_at, uj.applied_at)
            WHEN 'dismissed' THEN uj.dismissed_at
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

-- name: ListDismissedJobSlugs :many
-- Every public_slug the user has hidden (dismissed). Used by the SPA to exclude
-- hidden jobs from the browse feed client-side, mirroring ListSavedJobSlugs —
-- cross-referenced in the browser, never joined into ListJobs/SearchJobs. Bounded
-- by the caller's dismissed subset (typically small) and indexed by the
-- (user_id, job_id) primary key. Closed jobs are included: a hidden posting that
-- later closes stays hidden.
SELECT jobs.public_slug
FROM user_jobs uj
JOIN jobs ON jobs.id = uj.job_id
WHERE uj.user_id = $1 AND uj.dismissed_at IS NOT NULL;

-- name: CountUserJobs :one
-- Per-filter row counts for the my-jobs tabs, in one aggregate pass. "all" is
-- every interaction row; "viewed" is the view-only subset (neither saved nor
-- applied), matching the ListUserJobs filter. "board" counts jobs on the Kanban
-- board (saved, applied, or stage set), matching the ListUserJobs board filter.
-- "dismissed" counts jobs the user hid from the feed, matching the ListUserJobs
-- dismissed filter.
SELECT count(*)                                        AS "all",
       count(*) FILTER (WHERE saved_at IS NULL
                          AND applied_at IS NULL)      AS viewed,
       count(*) FILTER (WHERE saved_at   IS NOT NULL) AS saved,
       count(*) FILTER (WHERE applied_at IS NOT NULL) AS applied,
       count(*) FILTER (WHERE saved_at   IS NOT NULL
                            OR applied_at IS NOT NULL
                            OR stage      IS NOT NULL) AS board,
       count(*) FILTER (WHERE dismissed_at IS NOT NULL) AS dismissed
FROM user_jobs
WHERE user_id = $1;

-- name: LockJobForApply :exec
-- Take the job row's lock so concurrent applies on the same job serialize. Without
-- it, two applies in the same window under READ COMMITTED each read a snapshot
-- missing the other's user_jobs row and both bump applied_count. Called first in
-- the apply transaction, before MarkJobApplied (same pattern as LockJobForVote).
SELECT 1 FROM jobs WHERE id = $1 FOR UPDATE;

-- name: LockJobForVote :exec
-- Take the job row's lock so concurrent votes on the same job serialize. Without
-- it, two votes in the same window under READ COMMITTED each recompute against a
-- snapshot missing the other's user_jobs row, permanently undercounting the target
-- until the next uncontended vote. Called first in the vote transaction.
SELECT 1 FROM jobs WHERE id = $1 FOR UPDATE;

-- name: UpsertJobVote :one
-- Cast a thumbs vote on a job for a user with toggle/flip semantics: casting the
-- same direction again clears the vote (-> NULL), the opposite direction replaces it.
-- Upserts the (user, job) row (viewed_at defaults) so a vote can precede any other
-- interaction. $3 is the desired direction (-1 or 1). Returns the caller's resulting
-- vote (0 when the toggle cleared it). The caller recomputes jobs counters in the
-- same transaction via RecountJobVotes; a CTE cannot, since a data-modifying CTE's
-- effect is invisible to sibling sub-statements.
WITH upsert AS (
    INSERT INTO user_jobs (user_id, job_id, vote)
    VALUES ($1, $2, $3)
    ON CONFLICT (user_id, job_id) DO UPDATE
      SET vote = CASE WHEN user_jobs.vote = EXCLUDED.vote THEN NULL ELSE EXCLUDED.vote END
    RETURNING vote
)
SELECT COALESCE((SELECT vote FROM upsert), 0)::smallint AS my_vote;

-- name: ClearJobVote :exec
-- Explicitly clear a user's job vote (the DELETE endpoint). No-op when no row or no
-- vote exists. The caller recomputes counters via RecountJobVotes in the same tx.
UPDATE user_jobs SET vote = NULL WHERE user_id = $1 AND job_id = $2;

-- name: RecountJobVotes :one
-- Recompute a single job's materialized vote counters from user_jobs and return
-- them. Run as its own statement AFTER the vote write within one transaction, so it
-- sees the write (a same-statement CTE would not). Scoped to one job_id via the
-- partial index user_jobs(job_id) WHERE vote IS NOT NULL — a cheap indexed count.
UPDATE jobs SET
    upvote_count   = (SELECT count(*) FROM user_jobs uj WHERE uj.job_id = $1 AND uj.vote =  1),
    downvote_count = (SELECT count(*) FROM user_jobs uj WHERE uj.job_id = $1 AND uj.vote = -1)
WHERE id = $1
RETURNING upvote_count, downvote_count;

-- name: GetJobVote :one
-- The caller's current vote for a job (0 when none), for my_vote on auth-aware
-- detail reads. Always returns one row via the COALESCE'd scalar subquery.
SELECT COALESCE((SELECT vote FROM user_jobs WHERE user_id = $1 AND job_id = $2), 0)::smallint AS my_vote;

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

-- name: RecordApplicationFollowUp :one
-- Record that the candidate chased a silent application. Owner-scoped: a foreign or untracked job
-- matches no row, so the handler 404s and nothing is written. Idempotent by design — a double click
-- just overwrites the timestamp with a later one rather than erroring.
--
-- Only an application can be chased: a job merely viewed or saved has nobody to chase, which is why
-- applied_at must be set. This is NOT fed into the last-activity derivation above; see the column
-- comment in 0059 for why a chase must not clear the silence it was a response to.
-- The ledger records one `follow_up_sent` event per chase, which is what the single
-- column above cannot do: it holds the latest chase only, so the first is erased by the
-- second even though a second chase is the candidate's deliberate decision.
-- The column's own idempotence is the reason for the hour: a double submit arrives
-- seconds apart, a genuine second chase days apart, so any threshold between them is
-- safe. An hour is the smallest that sits comfortably above a retry.
WITH prior AS (
    SELECT uj.followed_up_at FROM user_jobs uj
     WHERE uj.user_id = $1 AND uj.job_id = $2 AND uj.applied_at IS NOT NULL
), upd AS (
    UPDATE user_jobs
    SET followed_up_at = now()
    WHERE user_id = $1 AND job_id = $2 AND applied_at IS NOT NULL
    RETURNING followed_up_at
), event AS (
    INSERT INTO application_events (user_id, job_id, company_slug, kind, occurred_at, source)
    SELECT $1, $2, j.company_slug, 'follow_up_sent', u.followed_up_at, sqlc.arg(event_source)::text
      FROM upd u JOIN jobs j ON j.id = $2
     WHERE NOT EXISTS (
         SELECT 1 FROM prior
          WHERE prior.followed_up_at IS NOT NULL
            AND prior.followed_up_at > now() - interval '1 hour'
     )
)
SELECT followed_up_at FROM upd;
