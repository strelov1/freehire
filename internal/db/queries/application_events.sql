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
ON CONFLICT (user_id, kind, source_ref) WHERE source_ref IS NOT NULL DO NOTHING;

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
