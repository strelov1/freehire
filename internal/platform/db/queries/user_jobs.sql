-- name: RecordJobView :one
-- Record (or refresh) a user's view of a job. Idempotent on (user_id, job_id):
-- the first view creates the row, a repeat view touches viewed_at. Returns the
-- row so the caller learns the current applied_at in the same round-trip. This
-- does NOT touch jobs.view_count — that materialized counter is maintained solely
-- by the nginx-log aggregation worker (cmd/rollup-views), which counts all traffic
-- (anonymous + signed-in + API), so the signed-in beacon must not double-count.
-- The interaction shape the caller has always received is now assembled from two
-- tables: user_jobs keeps the marks on the posting, applications holds the process.
-- The column list and its order mirror user_jobs' own, so the Go layer's
-- db.UserJob(row) conversion keeps working across every query in this file.
WITH touch AS (
    INSERT INTO user_jobs (user_id, job_id)
    VALUES ($1, $2)
    ON CONFLICT (user_id, job_id) DO UPDATE SET viewed_at = now()
    RETURNING *
)
SELECT t.user_id, t.job_id, t.viewed_at, a.applied_at, t.saved_at,
       a.stage, a.notes, t.dismissed_at, t.vote, a.followed_up_at
  FROM touch t
  LEFT JOIN applications a ON a.user_id = t.user_id AND a.job_id = t.job_id;

-- name: MarkJobApplied :one
-- Mark a job as applied for a user. Idempotent and independent of a prior view:
-- it inserts the row (viewed_at defaults) or updates applied_at in place, and
-- seeds stage='applied' when the stage is unset OR still 'preparing' — CV
-- tailoring's board placement (internal/api/handler/cv.go's EnsureOnBoard) sets
-- 'preparing' with no applied_at, and a real apply signal is exactly the event
-- that should promote it. Any other existing stage survives a re-apply
-- unchanged. When (and only when) applied_at transitions from
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
    SELECT a.applied_at FROM applications a WHERE a.user_id = sqlc.arg(user_id) AND a.job_id = sqlc.arg(job_id)::bigint
), touch AS (
    -- The interaction row still exists for its own duties; applying to a job never
    -- opened must still leave one behind, as it always has.
    INSERT INTO user_jobs (user_id, job_id)
    VALUES (sqlc.arg(user_id), sqlc.arg(job_id))
    ON CONFLICT (user_id, job_id) DO UPDATE SET user_id = user_jobs.user_id
    RETURNING *
), upsert AS (
    INSERT INTO applications (user_id, job_id, company_slug, role_title, applied_at, stage)
    SELECT sqlc.arg(user_id), sqlc.arg(job_id)::bigint, j.company_slug, j.title,
           COALESCE(sqlc.narg(at)::timestamptz, now()), 'applied'
      FROM jobs j WHERE j.id = sqlc.arg(job_id)::bigint
    ON CONFLICT (user_id, job_id) WHERE job_id IS NOT NULL
    DO UPDATE
      SET applied_at = CASE
              WHEN sqlc.narg(at)::timestamptz IS NOT NULL
                  THEN COALESCE(applications.applied_at, sqlc.narg(at)::timestamptz)
              ELSE now()
          END,
          stage = CASE
              WHEN applications.stage IS NULL OR applications.stage = 'preparing' THEN 'applied'
              ELSE applications.stage
          END
    RETURNING *
), bump AS (
    UPDATE jobs SET applied_count = applied_count + 1
    WHERE id = sqlc.arg(job_id) AND NOT EXISTS (SELECT 1 FROM prior WHERE prior.applied_at IS NOT NULL)
), event AS (
    INSERT INTO application_events (user_id, application_id, job_id, company_slug, kind, occurred_at, source)
    SELECT sqlc.arg(user_id), u.id, sqlc.arg(job_id)::bigint, j.company_slug, 'applied', u.applied_at, sqlc.arg(event_source)::text
      FROM upsert u JOIN jobs j ON j.id = sqlc.arg(job_id)::bigint
     WHERE NOT EXISTS (SELECT 1 FROM prior WHERE prior.applied_at IS NOT NULL)
)
SELECT t.user_id, t.job_id, t.viewed_at, u.applied_at, t.saved_at,
       u.stage, u.notes, t.dismissed_at, t.vote, u.followed_up_at, u.id AS application_id
  FROM touch t, upsert u;

-- name: SaveJob :one
-- Save (bookmark) a job for a user. Idempotent and independent of a prior view:
-- it inserts the row (viewed_at defaults) or refreshes saved_at in place.
WITH touch AS (
    INSERT INTO user_jobs (user_id, job_id, saved_at)
    VALUES ($1, $2, now())
    ON CONFLICT (user_id, job_id) DO UPDATE SET saved_at = now()
    RETURNING *
)
SELECT t.user_id, t.job_id, t.viewed_at, a.applied_at, t.saved_at,
       a.stage, a.notes, t.dismissed_at, t.vote, a.followed_up_at
  FROM touch t
  LEFT JOIN applications a ON a.user_id = t.user_id AND a.job_id = t.job_id;

-- name: UnsaveJob :one
-- Clear a job's saved mark without deleting the interaction row, so view and
-- apply history survive unsaving. No interaction row -> pgx.ErrNoRows; the
-- handler treats that as "already not saved", never as a failure.
WITH touch AS (
    UPDATE user_jobs SET saved_at = NULL
    WHERE user_jobs.user_id = $1 AND user_jobs.job_id = $2
    RETURNING *
)
SELECT t.user_id, t.job_id, t.viewed_at, a.applied_at, t.saved_at,
       a.stage, a.notes, t.dismissed_at, t.vote, a.followed_up_at
  FROM touch t
  LEFT JOIN applications a ON a.user_id = t.user_id AND a.job_id = t.job_id;

-- name: DismissJob :one
-- Dismiss (swipe away) a job for a user in the swipe deck. Idempotent and
-- independent of a prior view: it inserts the row (viewed_at defaults) or
-- refreshes dismissed_at in place.
WITH touch AS (
    INSERT INTO user_jobs (user_id, job_id, dismissed_at)
    VALUES ($1, $2, now())
    ON CONFLICT (user_id, job_id) DO UPDATE SET dismissed_at = now()
    RETURNING *
)
SELECT t.user_id, t.job_id, t.viewed_at, a.applied_at, t.saved_at,
       a.stage, a.notes, t.dismissed_at, t.vote, a.followed_up_at
  FROM touch t
  LEFT JOIN applications a ON a.user_id = t.user_id AND a.job_id = t.job_id;

-- name: UndismissJob :one
-- Clear a job's dismissed mark without deleting the interaction row, so view/
-- apply/save history survives. No interaction row -> pgx.ErrNoRows; the handler
-- treats that as "already not dismissed", never as a failure. This is the undo
-- path for a swipe-left decision.
WITH touch AS (
    UPDATE user_jobs SET dismissed_at = NULL
    WHERE user_jobs.user_id = $1 AND user_jobs.job_id = $2
    RETURNING *
)
SELECT t.user_id, t.job_id, t.viewed_at, a.applied_at, t.saved_at,
       a.stage, a.notes, t.dismissed_at, t.vote, a.followed_up_at
  FROM touch t
  LEFT JOIN applications a ON a.user_id = t.user_id AND a.job_id = t.job_id;

-- name: ExcludedJobIDs :many
-- Job ids the user has already interacted with (viewed, saved, applied, or
-- dismissed) — the swipe deck's exclusion set, so a card is shown at most once
-- across sessions. viewed_at is set on every interaction row, so any row for the
-- user counts (the deck records a view the moment a card is shown). Ordered
-- most-recently-touched first and capped ($2) so the deck's `id NOT IN (...)`
-- search filter stays bounded; the overflow risk is only an occasional re-shown
-- long-ago-seen job, never a correctness problem.
SELECT uj.job_id
FROM user_jobs uj
LEFT JOIN applications a ON a.user_id = uj.user_id AND a.job_id = uj.job_id
WHERE uj.user_id = $1
ORDER BY GREATEST(uj.viewed_at, uj.saved_at, a.applied_at, uj.dismissed_at) DESC
LIMIT $2;

-- name: FeedExcludedJobIDs :many
-- Job ids the match feed leaves out for a paging snapshot: saved, dismissed or applied
-- strictly before `before`, so an action taken after the snapshot cannot shift a window the
-- reader is paging through. Viewed jobs and tracked-but-unapplied applications stay. Newest action first and capped (`row_limit`) so the
-- search's `id NOT IN (...)` filter stays bounded.
SELECT job_id FROM (
    SELECT uj.job_id AS job_id, uj.saved_at AS at FROM user_jobs uj
     WHERE uj.user_id = sqlc.arg(user_id) AND uj.saved_at < sqlc.arg(before)::timestamptz
    UNION ALL
    SELECT uj.job_id, uj.dismissed_at FROM user_jobs uj
     WHERE uj.user_id = sqlc.arg(user_id) AND uj.dismissed_at < sqlc.arg(before)::timestamptz
    UNION ALL
    SELECT a.job_id, a.applied_at FROM applications a
     WHERE a.user_id = sqlc.arg(user_id) AND a.job_id IS NOT NULL AND a.applied_at < sqlc.arg(before)::timestamptz
) x
GROUP BY job_id
ORDER BY max(at) DESC
LIMIT sqlc.arg(row_limit);

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
-- employer), which is why nothing times it yet — see internal/application/appevent.
WITH prior AS (
    SELECT a.stage FROM applications a WHERE a.user_id = sqlc.arg(user_id) AND a.job_id = sqlc.arg(job_id)::bigint
), touch AS (
    -- The interaction row keeps its own duties; tracking a job never opened must
    -- still leave one behind.
    INSERT INTO user_jobs (user_id, job_id)
    VALUES (sqlc.arg(user_id), sqlc.arg(job_id))
    ON CONFLICT (user_id, job_id) DO UPDATE SET user_id = user_jobs.user_id
    RETURNING *
), upsert AS (
    -- applied_at is deliberately not set: setting a stage is not applying, and a
    -- record with no date reads as "tracked, not recorded as applied" (0065).
    INSERT INTO applications (user_id, job_id, company_slug, role_title, stage, notes)
    SELECT sqlc.arg(user_id), sqlc.arg(job_id)::bigint, j.company_slug, j.title, sqlc.narg(stage), sqlc.narg(notes) FROM jobs j WHERE j.id = sqlc.arg(job_id)::bigint::bigint
    ON CONFLICT (user_id, job_id) WHERE job_id IS NOT NULL
    DO UPDATE SET stage = COALESCE(EXCLUDED.stage, applications.stage),
                  notes = COALESCE(EXCLUDED.notes, applications.notes)
    RETURNING *
), event AS (
    -- The event names the application, like every other kind does. Resolved through the
    -- posting because that is the link the caller has, and soundly so: this runs at write
    -- time, while the posting is still there. A stage set on a job never applied to has
    -- no application to name and leaves the column NULL — there is nothing to correlate
    -- yet, and inventing a record would assert an application that was never made.
    -- `upsert` IS the application now, so its id is in hand rather than resolved
    -- back through the posting.
    INSERT INTO application_events (user_id, application_id, job_id, company_slug, kind, signal, occurred_at, source)
    SELECT sqlc.arg(user_id), u.id, sqlc.arg(job_id)::bigint, j.company_slug, 'stage_set', u.stage, now(), sqlc.arg(event_source)::text
      FROM upsert u JOIN jobs j ON j.id = sqlc.arg(job_id)::bigint
     WHERE u.stage IS NOT NULL
       AND u.stage IS DISTINCT FROM (SELECT prior.stage FROM prior)
)
SELECT t.user_id, t.job_id, t.viewed_at, u.applied_at, t.saved_at,
       u.stage, u.notes, t.dismissed_at, t.vote, u.followed_up_at
  FROM touch t, upsert u;

-- name: ClearJobProgress :one
-- Reset a tracked job to the wishlist: drop stage and applied state, keep saved/viewed/notes.
-- The application record stays: it holds the notes, and clearing progress is the
-- candidate reconsidering, not a claim the process never happened.
WITH cleared AS (
    UPDATE applications SET stage = NULL, applied_at = NULL
    WHERE applications.user_id = sqlc.arg(user_id) AND applications.job_id = sqlc.arg(job_id)
    RETURNING *
)
SELECT uj.user_id, uj.job_id, uj.viewed_at, c.applied_at, uj.saved_at,
       c.stage, c.notes, uj.dismissed_at, uj.vote, c.followed_up_at
  FROM user_jobs uj
  LEFT JOIN cleared c ON c.user_id = uj.user_id AND c.job_id = uj.job_id
 WHERE uj.user_id = sqlc.arg(user_id) AND uj.job_id = sqlc.arg(job_id);

-- name: UntrackJob :one
-- Remove a job from the board: drop every pipeline mark, keep viewed_at so the
-- job remains in the user's view history.
-- Taking a job off the board clears the process outright — this is the candidate
-- saying it is not a thing they are pursuing, which is a claim about their own
-- record and theirs alone to make. A catalogue clean-up has no such standing, which is the
-- whole distinction this change draws.
WITH dropped AS (
    DELETE FROM applications WHERE applications.user_id = sqlc.arg(user_id) AND applications.job_id = sqlc.arg(job_id)::bigint
), unsaved AS (
    UPDATE user_jobs SET saved_at = NULL
    WHERE user_jobs.user_id = sqlc.arg(user_id) AND user_jobs.job_id = sqlc.arg(job_id)
    RETURNING *
)
SELECT u.user_id, u.job_id, u.viewed_at,
       NULL::timestamptz AS applied_at, u.saved_at,
       NULL::text        AS stage,
       NULL::text        AS notes,
       u.dismissed_at, u.vote,
       NULL::timestamptz AS followed_up_at
  FROM unsaved u;

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

-- name: GetUserJobApplied :one
-- Whether the caller already applied to a job (applications.applied_at set — the process
-- table, not user_jobs, holds this column; see RecordJobView's own comment above), the
-- durable signal cmd/auto-apply/store.go's Submit stamps via MarkJobApplied on a real ATS
-- submission. The guard PostJobAutoApply consults so a re-click after a successful
-- auto-apply cannot start a second one; auto_apply_queue's own row is gone by then (Submit
-- deletes it in the same transaction). Same COALESCE'd-scalar-subquery idiom as GetJobVote,
-- for the same reason: always exactly one row, so a miss reads as "not applied" rather than
-- needing its own pgx.ErrNoRows branch.
SELECT ((SELECT applied_at FROM applications WHERE user_id = $1 AND job_id = $2) IS NOT NULL)::boolean AS applied;
