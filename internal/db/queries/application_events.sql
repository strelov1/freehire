-- name: ListApplicationEventsInRange :many
-- One caller's live events over a date range, oldest first — the ledger's first dated
-- read, behind internal/apptimeline and the tracking calendar.
--
-- Retracted rows are excluded here as they are in every other reader: a correction the
-- calendar still showed under the wrong employer would be a correction its author cannot
-- see they made.
--
-- The employer is taken from the event's own denormalized slug, never through the posting.
-- cmd/prune clears job_id, and an event that had to join jobs for its company would drop
-- out of the range retroactively — the instability the ledger exists to remove. The
-- posting is joined only for the slug the SPA links with, and legitimately comes back
-- absent.
--
-- The message is joined for its subject and nothing else. Its deletion is a condition OF
-- the join rather than a filter on the result, so a deleted message yields NULL on both
-- columns while the event itself stands: deletion hides content, it does not un-happen
-- the reply. Reading a body instead would mean GET /me/emails/:id, which marks mail read.
--
-- The join is restricted to mail-derived sources because source_ref names an emails.id
-- only for those — the column's comment in 0062 says so, and the idempotency index keys on
-- (user_id, kind, source_ref) precisely because the referent is namespaced per kind. On the
-- bare column, the next kind to carry a source_ref into some other table would be served
-- whichever of the caller's messages happened to share that id, and the calendar would
-- caption an interview with an unrelated rejection. The three names arrive as parameters
-- rather than being written here, so the vocabulary stays in Go where a pin test guards it.
SELECT ae.id, ae.kind, ae.signal, ae.source, ae.occurred_at, ae.company_slug,
       ae.application_id,
       a.role_title,
       j.public_slug AS job_slug,
       em.id         AS email_id,
       em.subject    AS email_subject
  FROM application_events ae
  LEFT JOIN applications a ON a.id = ae.application_id
                          AND a.user_id = ae.user_id
  LEFT JOIN jobs j         ON j.id = ae.job_id
  LEFT JOIN emails em      ON em.id = ae.source_ref
                          AND em.user_id = ae.user_id
                          AND em.deleted_at IS NULL
                          AND ae.source IN (sqlc.arg(src_gmail), sqlc.arg(src_hosted), sqlc.arg(src_external))
 WHERE ae.user_id      = $1
   AND ae.retracted_at IS NULL
   AND ae.occurred_at >= sqlc.arg(from_at)
   AND ae.occurred_at <= sqlc.arg(to_at)
 -- id breaks the tie so a day's events keep a stable order between requests; two events
 -- sharing a timestamp is routine when a mailbox import lands a batch.
 ORDER BY ae.occurred_at, ae.id;

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
  -- The PAIR, not either half. On the application alone this is blind to mail that
  -- names no application — re-linking it between two postings would move nothing. On
  -- the posting alone, cmd/prune clears both sides and the row reads as "moved
  -- elsewhere", retracting a fact nobody corrected. Row-wise IS DISTINCT FROM gets the
  -- NULLs right in both directions: a pruned pair (app, NULL) equals itself, and
  -- (NULL, jobA) differs from (NULL, jobB).
  AND (ae.application_id, ae.job_id) IS DISTINCT FROM (em.application_id, em.job_id);

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
-- The application the reply belongs to is what the aggregates pair on, so it is resolved
-- here rather than left to be inferred later from a job_id that cmd/prune clears. The
-- message's own application_id wins; falling back to the posting is sound only because
-- this runs at write time, while the posting still exists.
INSERT INTO application_events (
    user_id, application_id, job_id, company_slug, kind, signal, occurred_at, source, source_ref
)
-- The employer comes from the application when the message names one, and from the
-- posting otherwise: a reply to a job the candidate never recorded applying to is still
-- a fact worth keeping, it simply has no application to be paired with and never enters
-- the response ratio, which counts only replies that answer an `applied` event.
SELECT em.user_id, em.application_id, em.job_id,
       COALESCE(a.company_slug, j.company_slug, ''), 'employer_reply',
       COALESCE(em.status_signal, ''), em.received_at, sqlc.arg(event_source)::text, em.id
  FROM emails em
  LEFT JOIN applications a ON a.id = em.application_id
  LEFT JOIN jobs j ON j.id = em.job_id
 WHERE em.id      = $1
   AND em.user_id = $2
   AND (em.application_id IS NOT NULL OR em.job_id IS NOT NULL)
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

-- name: LastStageSetAt :one
-- When the candidate last set this application's stage themselves, or NULL if never.
--
-- This is what silences a mail-driven stage suggestion. A `stage_set` later than the message
-- that prompted one means the question has already been answered — whichever stage they chose,
-- including choosing to leave it where it was and then moving it somewhere else entirely. The
-- alternative, a dismissal flag on the email, would be a second store of a decision this ledger
-- already records, and the two would eventually disagree.
--
-- Retracted rows are excluded, and the (user_id, job_id, kind) index is partial on exactly that
-- predicate.
SELECT max(occurred_at)::timestamptz AS last_stage_set_at
  FROM application_events
 WHERE user_id = $1
   AND job_id = $2
   AND kind = 'stage_set'
   AND retracted_at IS NULL;

-- name: ListApplicationEvents :many
-- One application's live events, newest first — what the application panel renders as its
-- history, where ListApplicationEventsInRange paints a month for the calendar.
--
-- Same columns, same joins and the same retraction rule as that range read, deliberately: the
-- two answer different questions about the same ledger, and a row that meant one thing on the
-- calendar and another in the panel would be the drift this table exists to remove. See that
-- query for why the employer comes from the event's own slug, why the message is joined for
-- its subject alone, and why the join is restricted to mail-derived sources.
--
-- Newest first because it is a history: the reader wants what just happened, not what started
-- it. `id` breaks ties so a batch landing on one timestamp keeps a stable order.
--
-- Served by application_events_app_idx (user_id, job_id, kind) WHERE retracted_at IS NULL —
-- the predicate here is that index's leading pair.
SELECT ae.id, ae.kind, ae.signal, ae.source, ae.occurred_at, ae.company_slug,
       ae.application_id,
       a.role_title,
       j.public_slug AS job_slug,
       em.id         AS email_id,
       em.subject    AS email_subject
  FROM application_events ae
  LEFT JOIN applications a ON a.id = ae.application_id
                          AND a.user_id = ae.user_id
  LEFT JOIN jobs j         ON j.id = ae.job_id
  LEFT JOIN emails em      ON em.id = ae.source_ref
                          AND em.user_id = ae.user_id
                          AND em.deleted_at IS NULL
                          AND ae.source IN (sqlc.arg(src_gmail), sqlc.arg(src_hosted), sqlc.arg(src_external))
 WHERE ae.user_id      = $1
   AND ae.job_id       = $2
   AND ae.retracted_at IS NULL
 ORDER BY ae.occurred_at DESC, ae.id DESC
 LIMIT $3;
