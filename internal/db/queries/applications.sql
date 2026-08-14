-- Queries over the application record — the durable half of what used to live in
-- user_jobs. See migrations/0064_applications.sql for why it is a table of its own.

-- name: BackfillApplicationEventLinks :execrows
-- Point one batch of pre-existing ledger events at the application they belong to.
-- Every event recorded before this change names a posting and no application, and left
-- that way it stays correlated through job_id — the reference cmd/prune clears.
--
-- Matched on (user_id, job_id) because that is the only identity those rows have. It is
-- exactly the correlation being retired, and it is sound here and nowhere else: this pass
-- runs before any posting has been cleared out from under the events it is repairing, and
-- a row whose job_id is already NULL is skipped rather than guessed at.
WITH page AS (
    SELECT ae.id, a.id AS application_id
      FROM application_events ae
      JOIN applications a ON a.user_id = ae.user_id AND a.job_id = ae.job_id
     WHERE ae.application_id IS NULL
       AND ae.job_id IS NOT NULL
     ORDER BY ae.id
     LIMIT sqlc.arg(batch_size)
)
UPDATE application_events ae
   SET application_id = p.application_id
  FROM page p
 WHERE ae.id = p.id;

-- name: BackfillEmailApplicationLinks :execrows
-- The same for mail. A thread linked to a posting must end up linked to the application,
-- or the first deletion detaches it from a record that is still standing.
WITH page AS (
    SELECT e.id, a.id AS application_id
      FROM emails e
      JOIN applications a ON a.user_id = e.user_id AND a.job_id = e.job_id
     WHERE e.application_id IS NULL
       AND e.job_id IS NOT NULL
     ORDER BY e.id
     LIMIT sqlc.arg(batch_size)
)
UPDATE emails e
   SET application_id = p.application_id
  FROM page p
 WHERE e.id = p.id;

-- name: ListOrphanedApplications :many
-- The caller's applications whose posting the catalogue no longer holds.
--
-- Deliberately joins nothing: cmd/prune cleared job_id, so there is no posting to reach
-- and sqlc.embed over a LEFT JOIN would generate a non-pointer Job that fails at scan
-- time on the NULL columns (measured). The employer and role title are on the record
-- itself, which is the whole reason they were copied there.
--
-- The board reads these alongside the posting-backed rows and merges the two; they are
-- few by nature — one appears only when a posting a candidate applied to is pruned.
--
-- Takes OFFSET as well as LIMIT so a caller paging the merged board (ListInteractions)
-- can advance past the first page of orphans instead of re-reading the same top-N rows
-- on every page — the query alone cannot fix that, since ListInteractions decides how
-- much of the requested (limit, offset) window belongs to the posting-backed rows
-- versus the orphaned ones.
SELECT a.id, a.company_slug, a.role_title, a.applied_at, a.stage, a.notes, a.followed_up_at,
       (SELECT count(*)
          FROM emails e
         WHERE e.application_id = a.id
           AND e.deleted_at IS NULL) AS email_count,
       -- Same last-activity rule as the posting-backed rows: the apply date, or the newest
       -- linked message when that is later. Mail is reached through the application, never
       -- through the posting, which no longer exists.
       (CASE WHEN a.applied_at IS NOT NULL THEN
          GREATEST(a.applied_at,
                   (SELECT max(e.received_at)
                      FROM emails e
                     WHERE e.application_id = a.id
                       AND e.deleted_at IS NULL))
        END)::timestamptz AS last_activity_at
  FROM applications a
 WHERE a.user_id = $1
   AND a.job_id IS NULL
 ORDER BY a.applied_at DESC NULLS LAST, a.id DESC
 LIMIT $2 OFFSET $3;

-- name: CountOrphanedApplications :one
-- How many of the caller's applications have no posting left — see
-- ListOrphanedApplications. CountUserJobs is driven entirely FROM user_jobs, which
-- cmd/prune cascades away for a pruned posting, so it never counts these; this is added
-- to its "all"/"applied"/"board" totals by the caller.
SELECT count(*) FROM applications WHERE user_id = $1 AND job_id IS NULL;

-- name: TrackApplicationByID :one
-- Set an application's stage and/or notes, naming the application itself.
--
-- The slug-addressed TrackJob cannot serve an application whose posting cmd/prune
-- removed: it upserts through `jobs`, and there is no row left to join. That is not a
-- corner case on the board — the card is there, and dragging it is the ordinary act
-- that had no working write path.
--
-- Partial update and the stage_set ledger event on a real transition, both exactly as
-- TrackJob does them: `prior` reads the pre-update value, so re-setting the stage a row
-- already carries, or a notes-only call, records nothing.
WITH prior AS (
    SELECT a.stage FROM applications a
     WHERE a.id = sqlc.arg(id) AND a.user_id = sqlc.arg(user_id)
), upd AS (
    UPDATE applications
       SET stage = COALESCE(sqlc.narg(stage), applications.stage),
           notes = COALESCE(sqlc.narg(notes), applications.notes)
     WHERE applications.id = sqlc.arg(id) AND applications.user_id = sqlc.arg(user_id)
    RETURNING *
), event AS (
    -- job_id and company_slug come off the application, not off a posting: this row may
    -- have no posting at all, and the employer is denormalised here for that reason.
    INSERT INTO application_events (user_id, application_id, job_id, company_slug, kind, signal, occurred_at, source)
    SELECT sqlc.arg(user_id), u.id, u.job_id, u.company_slug, 'stage_set', u.stage, now(), sqlc.arg(event_source)::text
      FROM upd u
     WHERE u.stage IS NOT NULL
       AND u.stage IS DISTINCT FROM (SELECT prior.stage FROM prior)
)
SELECT u.job_id, u.applied_at, u.stage, u.notes, u.followed_up_at FROM upd u;

-- name: ClearApplicationProgressByID :one
-- Drop an application's pipeline progress while keeping the record — the notes are the
-- candidate's own text, and reconsidering is not a claim the process never happened.
-- Clearing both stage and applied_at is what takes it off the board (see columnOf).
UPDATE applications
   SET stage = NULL, applied_at = NULL
 WHERE applications.id = sqlc.arg(id) AND applications.user_id = sqlc.arg(user_id)
RETURNING job_id, applied_at, stage, notes, followed_up_at;

-- name: UntrackApplicationByID :one
-- Take an application off the board outright, naming the application itself.
--
-- Deletes the record, matching UntrackJob: this is the candidate saying it is not a
-- thing they are pursuing, which is a claim about their own record and theirs alone to
-- make. cmd/prune has no such standing, which is why it may not do this.
DELETE FROM applications
 WHERE applications.id = sqlc.arg(id) AND applications.user_id = sqlc.arg(user_id)
RETURNING job_id, applied_at, stage, notes, followed_up_at;

-- name: CountPreparingBackfillCandidates :one
-- Read-only counterpart to BackfillPreparingStage, for cmd/backfill-preparing-stage's
-- --dry-run: how many applications the correction below would touch, without touching them.
-- Kept in exact lockstep with that query's WHERE clause deliberately — see its comment for
-- what the shape means and why the EXISTS join is the evidence, not the WHERE alone.
SELECT count(*) AS count
  FROM applications a
 WHERE a.stage = 'applied'
   AND a.applied_at IS NULL
   AND EXISTS (
     SELECT 1 FROM cvs c
      WHERE c.user_id = a.user_id AND c.job_id = a.job_id AND c.job_id IS NOT NULL
   );

-- name: BackfillPreparingStage :one
-- One-time correction for applications the pre-preparing-stage EnsureOnBoard wrote as
-- stage='applied' with no applied_at (see cv-tailoring's board placement in
-- internal/handler/cv.go, which now writes 'preparing' directly — this repairs what it wrote
-- before that changed).
--
-- The WHERE clause alone cannot tell tailoring's placement apart from a candidate's own
-- manual, undated drag into the Applied column, which produces the identical
-- stage='applied'/applied_at=NULL shape (see JobBoard.svelte's persistMove) and must NOT be
-- relabeled. A tailored CV on record for the same (user, job) is the strongest available
-- corroborating evidence of the former; the EXISTS join is that evidence, not a performance
-- detail.
--
-- Idempotent: a second run matches nothing, because a row this statement (or the current
-- EnsureOnBoard) already moved to 'preparing' no longer reads stage='applied'.
--
-- Writes a stage_set ledger event per corrected row, source='system': the platform is
-- correcting its own past write, not the candidate doing anything (see appevent.SourceSystem).
-- Returns the count of applications corrected.
WITH upd AS (
    UPDATE applications a
       SET stage = 'preparing'
     WHERE a.stage = 'applied'
       AND a.applied_at IS NULL
       AND EXISTS (
         SELECT 1 FROM cvs c
          WHERE c.user_id = a.user_id AND c.job_id = a.job_id AND c.job_id IS NOT NULL
       )
    RETURNING a.id, a.user_id, a.job_id, a.company_slug
), event AS (
    INSERT INTO application_events (user_id, application_id, job_id, company_slug, kind, signal, occurred_at, source)
    SELECT u.user_id, u.id, u.job_id, u.company_slug, 'stage_set', 'preparing', now(), 'system'
      FROM upd u
)
SELECT count(*) AS count FROM upd;
