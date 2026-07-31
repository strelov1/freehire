-- name: ListGhostApplicationEvidence :many
-- Candidate applications for the ghost signal, for a page of jobs at a time.
--
-- This query selects and gates; it does NOT judge. Whether an application is
-- actually silent is decided in Go by internal/userjob's threshold ladder, whose
-- five values carry their measured provenance. Restating that ladder here would let
-- a change to it disagree silently with the personal tracking board — the same
-- application judged by two ladders on two surfaces, with nothing binding them.
--
-- The mailbox gate is the one rule that DOES belong here, because it is a join.
-- jobtracking.Silence falls back to applied_at when an application has no linked
-- mail, which is right for the owner's own board and wrong as input to a public
-- claim: for a user with no connected mailbox there is never linked mail, so every
-- application of theirs reads silent once the threshold passes — including the ones
-- the employer answered somewhere we cannot see. Absence of a reply is evidence only
-- where a reply would have been observed.
--
-- last_activity_at and has_pending_suggestion mirror ListTrackedJobs deliberately:
-- one definition of "when did this application last move", not two.
-- job_id is nullable on applications, but the filter below matches it against a page of
-- real ids, so a row that reaches the select always carries one.
SELECT a.job_id::bigint AS job_id,
       a.user_id,
       coalesce(a.stage, '')::text AS stage,
       GREATEST(a.applied_at,
                (SELECT max(e.received_at)
                   FROM emails e
                  WHERE e.user_id = a.user_id
                    AND e.job_id = a.job_id
                    AND e.deleted_at IS NULL))::timestamptz AS last_activity_at,
       (EXISTS (SELECT 1
                  FROM emails e
                 WHERE e.user_id = a.user_id
                   AND e.suggested_job_id = a.job_id
                   -- "Pending" means the caller has not confirmed it, and confirming is
                   -- what attaches the application — the same test the inbox's link
                   -- filter makes, so the two cannot disagree about one message.
                   AND e.application_id IS NULL
                   AND e.deleted_at IS NULL))::boolean AS has_pending_suggestion
FROM applications a
WHERE a.job_id = ANY(sqlc.arg(job_ids)::bigint[])
  AND a.applied_at IS NOT NULL
  AND (EXISTS (SELECT 1 FROM gmail_connections gc
                WHERE gc.user_id = a.user_id AND gc.status = 'connected')
    OR EXISTS (SELECT 1 FROM mailboxes mb WHERE mb.user_id = a.user_id));

-- name: ListGhostReportEvidence :many
-- Non-retracted ghost reports for a page of jobs. Maturity — whether the stated
-- apply date has aged past the `applied` threshold — is applied in Go alongside the
-- silence ladder it reads from, so both channels clear the same bar from the same
-- source.
SELECT r.job_id, r.user_id, r.applied_on
FROM ghost_reports r
WHERE r.job_id = ANY(sqlc.arg(job_ids)::bigint[])
  AND r.retracted_at IS NULL;
