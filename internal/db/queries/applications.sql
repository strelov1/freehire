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
 LIMIT $2;
