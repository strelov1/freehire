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
    SELECT a.followed_up_at FROM applications a
     WHERE a.user_id = sqlc.arg(user_id) AND a.job_id = sqlc.arg(job_id)::bigint AND a.applied_at IS NOT NULL
), upd AS (
    UPDATE applications
    SET followed_up_at = now()
    WHERE applications.user_id = sqlc.arg(user_id) AND applications.job_id = sqlc.arg(job_id)::bigint
      AND applications.applied_at IS NOT NULL
    RETURNING id, followed_up_at
), event AS (
    INSERT INTO application_events (user_id, application_id, job_id, company_slug, kind, occurred_at, source)
    SELECT sqlc.arg(user_id), u.id, sqlc.arg(job_id)::bigint, j.company_slug, 'follow_up_sent', u.followed_up_at, sqlc.arg(event_source)::text
      FROM upd u JOIN jobs j ON j.id = sqlc.arg(job_id)::bigint
     WHERE NOT EXISTS (
         SELECT 1 FROM prior
          WHERE prior.followed_up_at IS NOT NULL
            AND prior.followed_up_at > now() - interval '1 hour'
     )
)
SELECT followed_up_at FROM upd;

-- name: RedateApplication :one
-- Correct when an application happened, in both places that record it.
--
-- MarkJobApplied above accepts a date, but deliberately keeps one already stored: a later
-- recording is not a later application, and its `at` comes from employer mail, which is an
-- upper bound rather than the candidate's own word. When the candidate states the date, theirs
-- wins — that is this statement, and keeping it separate is what leaves MarkJobApplied's
-- predicate (the applied_count bump and the ledger insert, decided together) untouched.
--
-- The `applied` event moves with the column because occurred_at IS the column, copied at write
-- time, and every aggregate reads the event. `recorded_at` stays put: when we learned of the
-- application is not what is being corrected. `applied_count` is not mentioned at all — the
-- application was already counted, and re-dating it is not a second one.
--
-- Only an application that carries a date is re-dated. A row tracking a stage on a job never
-- applied to has nothing to correct, and setting applied_at here would assert an application
-- that was never made (0065).
WITH app AS (
    UPDATE applications ap
       SET applied_at = sqlc.arg(at)::timestamptz
     WHERE ap.user_id = sqlc.arg(user_id)
       AND ap.job_id = sqlc.arg(job_id)::bigint
       AND ap.applied_at IS NOT NULL
    RETURNING ap.*
), event AS (
    UPDATE application_events e
       SET occurred_at = sqlc.arg(at)::timestamptz
      FROM app
     WHERE e.application_id = app.id
       AND e.kind = 'applied'
)
SELECT t.user_id, t.job_id, t.viewed_at, a.applied_at, t.saved_at,
       a.stage, a.notes, t.dismissed_at, t.vote, a.followed_up_at
  FROM app a
  JOIN user_jobs t ON t.user_id = a.user_id AND t.job_id = a.job_id;

-- name: GetApplicationStageForMirror :one
-- The candidate's own record of an application, as an employer's pipeline move needs it:
-- its stage, and who set that stage last — the source of the most recently WRITTEN live
-- stage_set event, ordered by id, not occurred_at: mail events are dated by the message's
-- received_at, so a stage mail set after the employer's can carry an earlier date. The event
-- counts only while its signal still IS the live stage, and — when that stage is 'applied',
-- the one an apply seeds — only if it was written after the newest live 'applied' event:
-- re-applying after clearing progress seeds 'applied' with no stage_set event, and an older
-- employer event whose signal was 'applied' must not then pass for the author of the
-- candidate's fresh record. An apply that keeps a stage already set (interview stays
-- interview) leaves its author in place. '' — nobody known — otherwise, or with no event. MirrorEmployerStage compares against
-- the same expression. No row: the candidate has no record of this application, took it off their board,
-- or cleared its progress (stage and applied_at both NULL, ClearJobProgress) — each a
-- statement that they are not pursuing it, which an employer's move must not undo.
SELECT a.stage,
       COALESCE((SELECT CASE WHEN e.signal = a.stage THEN e.source END
         FROM application_events e
         WHERE e.application_id = a.id AND e.kind = 'stage_set' AND e.retracted_at IS NULL
           AND (a.stage IS DISTINCT FROM 'applied'
                OR e.id > COALESCE((SELECT max(x.id) FROM application_events x
                                     WHERE x.application_id = a.id AND x.kind = 'applied'
                                       AND x.retracted_at IS NULL), 0))
         ORDER BY e.id DESC
         LIMIT 1), '')::text AS stage_source
FROM applications a
WHERE a.user_id = sqlc.arg(user_id) AND a.job_id = sqlc.arg(job_id)::bigint
  AND (a.stage IS NOT NULL OR a.applied_at IS NOT NULL);

-- name: MirrorEmployerStage :one
-- Sets the candidate's stage to what the employer's pipeline says, but only if it still
-- holds the stage the caller read (`expected`, NULL for none) AND that stage still has the
-- author the caller read (`expected_source`, GetApplicationStageForMirror's expression): a
-- compare-and-set on both, so a candidate who changed the stage in between — even away and
-- back to the same value — is never overwritten. Records the `stage_set`
-- ledger event with source 'employer' in the same statement. Never creates a record: an
-- application the candidate took off their board stays off it.
WITH moved AS (
    UPDATE applications
    SET stage = sqlc.arg(stage)::text
    WHERE applications.user_id = sqlc.arg(user_id)
      AND applications.job_id = sqlc.arg(job_id)::bigint
      AND applications.stage IS NOT DISTINCT FROM sqlc.narg(expected)::text
      AND COALESCE((SELECT CASE WHEN e.signal = applications.stage THEN e.source END
         FROM application_events e
         WHERE e.application_id = applications.id AND e.kind = 'stage_set' AND e.retracted_at IS NULL
           AND (applications.stage IS DISTINCT FROM 'applied'
                OR e.id > COALESCE((SELECT max(x.id) FROM application_events x
                                     WHERE x.application_id = applications.id AND x.kind = 'applied'
                                       AND x.retracted_at IS NULL), 0))
         ORDER BY e.id DESC
         LIMIT 1), '') = sqlc.arg(expected_source)::text
      AND (applications.stage IS NOT NULL OR applications.applied_at IS NOT NULL)
    RETURNING applications.id, applications.company_slug
), event AS (
    INSERT INTO application_events (user_id, application_id, job_id, company_slug, kind, signal, occurred_at, source)
    SELECT sqlc.arg(user_id), m.id, sqlc.arg(job_id)::bigint, m.company_slug, 'stage_set', sqlc.arg(stage)::text, now(), 'employer'
      FROM moved m
)
SELECT EXISTS (SELECT 1 FROM moved) AS moved;
