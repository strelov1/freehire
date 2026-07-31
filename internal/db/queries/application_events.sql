-- name: RecordApplicationEvent :exec
-- Append one application event.
--
-- Idempotent for mail-derived events by constraint rather than by coordination: the
-- partial unique index on (user_id, kind, source_ref) makes emission and backfill the
-- same operation, so cmd/classify-mail and cmd/backfill-application-events can meet on
-- the same email in any order and produce one row. Manual events pass source_ref NULL
-- and fall outside the index — two consecutive follow-ups are two facts, not a
-- duplicate.
--
-- occurred_at is the caller's to supply and is never now() for mail: it is the message's
-- own received_at, so importing a year of ATS mail on the day a mailbox is connected
-- does not report a year of replies arriving that day.
INSERT INTO application_events (
    user_id, job_id, company_slug, kind, signal, occurred_at, source, source_ref
)
VALUES ($1, sqlc.narg(job_id), $2, $3, $4, $5, $6, sqlc.narg(source_ref))
ON CONFLICT (user_id, kind, source_ref) WHERE source_ref IS NOT NULL AND retracted_at IS NULL
DO NOTHING;

-- name: RetractApplicationEventsBySourceRef :execrows
-- Retract the events a source record produced, because the fact turned out to belong to
-- a different employer.
--
-- Only a link correction calls this. Deleting the message must NOT: the two actions look
-- alike and mean opposite things — deletion says the candidate does not want to see the
-- message, re-linking says the fact belongs elsewhere. A wrong link left standing poisons
-- a named company's public response rate permanently.
--
-- The row survives, stamped rather than deleted: an event recorded in error is itself a
-- fact, and this table is append-only. Already-retracted rows are skipped so a repeated
-- correction cannot move the stamp forward.
UPDATE application_events
SET retracted_at = now()
WHERE user_id = $1
  AND kind = $2
  AND source_ref = $3
  AND retracted_at IS NULL;

-- name: ListApplicationEventsForUserJob :many
-- Every non-retracted event recorded against one application, oldest first. The follow-up
-- history a single overwritten column used to destroy is readable here.
SELECT * FROM application_events
WHERE user_id = $1 AND job_id = $2 AND retracted_at IS NULL
ORDER BY occurred_at, id;

-- name: RetractSupersededEmailEvent :execrows
-- Step 1 of reconciling one email with the ledger: retract the live event when the
-- message is no longer linked, or is now linked to a different application.
--
-- Deleting or hiding the message is NOT one of those conditions and is deliberately not
-- consulted: deletion says the reader does not want to see it, re-linking says the fact
-- belongs to another employer. Only the second is a claim about who replied.
--
-- This is a separate statement from RecordEmailApplicationEvent, and must run first.
-- Folding both into one statement with data-modifying CTEs looks tidier and is wrong:
-- CTEs all read the same pre-statement snapshot, so the insert's ON CONFLICT would still
-- see the row this retracts as live, conflict with it, and silently record nothing — the
-- correction would appear to succeed while leaving the wrong company credited.
UPDATE application_events ae
SET retracted_at = now()
FROM emails em
WHERE em.id            = $1
  AND em.user_id       = $2
  AND ae.user_id       = em.user_id
  AND ae.kind          = 'employer_reply'
  AND ae.source_ref    = em.id
  AND ae.retracted_at IS NULL
  AND ae.job_id IS DISTINCT FROM em.job_id;

-- name: RecordEmailApplicationEvent :exec
-- Step 2: record the event when the message is LINKED and no live event exists for it.
-- Run RetractSupersededEmailEvent first.
--
-- Linked, not linked-and-classified. Requiring a classification reads as the stricter,
-- safer rule and is the opposite: `external` mail — the tier a caller's own harness
-- pushes — is never classified server-side by design, so those users' replies would
-- never count and their employers would look more silent than they were. That is the
-- distortion this ledger exists to remove. The signal is detail about the reply, not
-- evidence that one arrived; an unclassified reply records an empty signal.
--
-- The message's own received_at is the date. now() would compress a year of imported
-- history into the day a mailbox was connected.
--
-- Idempotent by the partial unique index, so the classification worker, the manual link
-- paths and the backfill can all call it in any order and produce one row.
-- `event_source` is derived from the message's store by the caller, keeping that
-- vocabulary in Go where a pin test guards it.
INSERT INTO application_events (
    user_id, job_id, company_slug, kind, signal, occurred_at, source, source_ref
)
SELECT em.user_id, em.job_id, j.company_slug, 'employer_reply',
       COALESCE(em.status_signal, ''), em.received_at, sqlc.arg(event_source)::text, em.id
  FROM emails em JOIN jobs j ON j.id = em.job_id
 WHERE em.id      = $1
   AND em.user_id = $2
   AND em.job_id IS NOT NULL
ON CONFLICT (user_id, kind, source_ref) WHERE source_ref IS NOT NULL AND retracted_at IS NULL
DO NOTHING;

-- name: BackfillEmployerReplyEvents :one
-- Replay one keyset batch of already-linked mail into the ledger.
--
-- Deleted mail is included deliberately. Deletion hides content; it does not un-happen the
-- reply, and excluding it here would bake the very inbox-hygiene dependence this ledger
-- exists to remove into its starting contents.
--
-- The three appevent source names arrive as parameters rather than being written here, so
-- the vocabulary stays in Go where a pin test guards it. Mail from an unrecognised store
-- is skipped rather than defaulted: an unknown provenance must not read as observed.
WITH batch AS (
    SELECT em.id, em.user_id, em.job_id, em.status_signal, em.received_at, em.source
      FROM emails em
     WHERE em.id > $1
       AND em.job_id IS NOT NULL
     ORDER BY em.id
     LIMIT sqlc.arg(batch_size)
), ins AS (
    INSERT INTO application_events (
        user_id, job_id, company_slug, kind, signal, occurred_at, source, source_ref
    )
    SELECT b.user_id, b.job_id, j.company_slug, 'employer_reply', COALESCE(b.status_signal, ''), b.received_at,
           CASE b.source
               WHEN 'gmail'    THEN sqlc.arg(src_gmail)::text
               WHEN 'hosted'   THEN sqlc.arg(src_hosted)::text
               WHEN 'external' THEN sqlc.arg(src_external)::text
           END,
           b.id
      FROM batch b JOIN jobs j ON j.id = b.job_id
     WHERE b.source IN ('gmail', 'hosted', 'external')
    ON CONFLICT (user_id, kind, source_ref) WHERE source_ref IS NOT NULL AND retracted_at IS NULL
    DO NOTHING
    RETURNING 1
)
SELECT COALESCE(max(b.id), 0)::bigint AS last_id,
       count(*)::bigint               AS scanned,
       (SELECT count(*) FROM ins)::bigint AS inserted
  FROM batch b;

-- name: BackfillAppliedEvents :one
-- Replay one keyset batch of recorded applications. Keyed by job_id because user_jobs has
-- no surrogate key; the pass walks (user_id, job_id) pairs in job order per user.
--
-- Only applications carry a date. A row that was staged but never marked applied has
-- nothing to replay, and the ledger says nothing about it rather than inventing a day.
WITH batch AS (
    SELECT uj.user_id, uj.job_id, uj.applied_at, uj.followed_up_at
      FROM user_jobs uj
     WHERE (uj.user_id, uj.job_id) > (sqlc.arg(last_user_id)::bigint, sqlc.arg(last_job_id)::bigint)
       AND uj.applied_at IS NOT NULL
     ORDER BY uj.user_id, uj.job_id
     LIMIT sqlc.arg(batch_size)
), applied AS (
    INSERT INTO application_events (user_id, job_id, company_slug, kind, occurred_at, source)
    SELECT b.user_id, b.job_id, j.company_slug, 'applied', b.applied_at, sqlc.arg(event_source)::text
      FROM batch b JOIN jobs j ON j.id = b.job_id
     WHERE NOT EXISTS (
         SELECT 1 FROM application_events ae
          WHERE ae.user_id = b.user_id AND ae.job_id = b.job_id AND ae.kind = 'applied'
     )
    RETURNING 1
), chased AS (
    INSERT INTO application_events (user_id, job_id, company_slug, kind, occurred_at, source)
    SELECT b.user_id, b.job_id, j.company_slug, 'follow_up_sent', b.followed_up_at, sqlc.arg(event_source)::text
      FROM batch b JOIN jobs j ON j.id = b.job_id
     WHERE b.followed_up_at IS NOT NULL
       AND NOT EXISTS (
         SELECT 1 FROM application_events ae
          WHERE ae.user_id = b.user_id AND ae.job_id = b.job_id AND ae.kind = 'follow_up_sent'
       )
    RETURNING 1
)
-- The cursor is the LAST ROW of the batch's own order, not max() of each column
-- independently: the greatest user_id and the greatest job_id can belong to different
-- rows, and a cursor assembled from both would jump past rows that were never scanned.
SELECT COALESCE((SELECT b2.user_id FROM batch b2 ORDER BY b2.user_id DESC, b2.job_id DESC LIMIT 1), 0)::bigint AS last_user_id,
       COALESCE((SELECT b2.job_id  FROM batch b2 ORDER BY b2.user_id DESC, b2.job_id DESC LIMIT 1), 0)::bigint AS last_job_id,
       count(*)::bigint AS scanned,
       ((SELECT count(*) FROM applied) + (SELECT count(*) FROM chased))::bigint AS inserted
  FROM batch b;
