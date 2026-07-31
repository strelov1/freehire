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
-- Step 2: record the event when the message is both linked and classified and no live
-- event exists for it. Run RetractSupersededEmailEvent first.
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
   AND COALESCE(em.status_signal, '') <> ''
ON CONFLICT (user_id, kind, source_ref) WHERE source_ref IS NOT NULL AND retracted_at IS NULL
DO NOTHING;
