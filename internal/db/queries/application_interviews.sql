-- name: UpsertApplicationInterview :one
-- Record a meeting the sync attached to an application, or move the one already recorded,
-- and note in the ledger that the scheduling was observed.
--
-- The sync re-reads its whole window every run, so this must be idempotent: the unique
-- index on (user_id, ical_uid) makes a second sighting an update rather than a second row.
-- A meeting that moved carries a new time under the same identity, which is exactly what
-- the ledger could not have expressed.
--
-- The status is the matcher's tier rendered: `confirmed` when the invitation's identifier
-- attached it, `suggested` when only the title did. A confirmed meeting never falls back
-- to a suggestion; the identifier that linked it is a fact, and a later run that only
-- recognises the title has learned nothing new.
--
-- The ledger event rides in the same statement, the way MarkJobApplied's does, so the
-- appointment and the record of it being made cannot drift apart. Two things about it:
--
--   * It is dated by the OBSERVATION, not by the meeting. occurred_at means "when this
--     happened" and every day calculation reads it; a row dated in the future would turn
--     the record of a search into a schedule.
--   * source_ref is the interview's own id, which makes it idempotent by the ledger's
--     partial unique index — a re-sync, and a reschedule, add no second event. The
--     scheduling happened once.
WITH saved AS (
    INSERT INTO application_interviews (
        user_id, application_id, ical_uid, starts_at, ends_at, title, join_url, status, source
    )
    VALUES (
        sqlc.arg(user_id), sqlc.arg(application_id), sqlc.arg(ical_uid),
        sqlc.arg(starts_at), sqlc.narg(ends_at), sqlc.arg(title), sqlc.arg(join_url),
        sqlc.arg(status), sqlc.arg(source)
    )
    ON CONFLICT (user_id, ical_uid) DO UPDATE
    SET application_id = EXCLUDED.application_id,
        starts_at      = EXCLUDED.starts_at,
        ends_at        = EXCLUDED.ends_at,
        title          = EXCLUDED.title,
        join_url       = EXCLUDED.join_url,
        status         = CASE
                             WHEN application_interviews.status = 'confirmed' THEN 'confirmed'
                             ELSE EXCLUDED.status
                         END,
        updated_at     = now()
    RETURNING id, user_id, application_id
), noted AS (
    INSERT INTO application_events (
        user_id, application_id, job_id, company_slug, kind, signal, occurred_at, source, source_ref
    )
    SELECT s.user_id, s.application_id, a.job_id, a.company_slug,
           'interview_scheduled', '', now(), sqlc.arg(event_source)::text, s.id
      FROM saved s JOIN applications a ON a.id = s.application_id
    ON CONFLICT (user_id, kind, source_ref) WHERE source_ref IS NOT NULL AND retracted_at IS NULL
    DO NOTHING
)
SELECT id FROM saved;

-- name: CancelApplicationInterview :execrows
-- Mark a meeting cancelled. Deliberately not a delete: a candidate who remembers an
-- interview on Thursday and finds an empty Thursday cannot tell a cancellation from a
-- calendar that failed to load. The scheduling still happened, and the ledger's
-- `interview_scheduled` stands.
UPDATE application_interviews
SET status = 'cancelled', updated_at = now()
WHERE user_id  = sqlc.arg(user_id)
  AND ical_uid = sqlc.arg(ical_uid)
  AND status  <> 'cancelled';

-- name: ListCalendarMatchCandidates :many
-- The caller's applications a meeting could belong to, each with the identifiers of the
-- invitations already linked to it.
--
-- One query per candidate rather than one lookup per calendar event: a sync window holds
-- far more events than a person has applications, and internal/calmatch is pure over what
-- it is handed. The UID array is what makes the deterministic tier possible — the mail
-- matcher already tied those invitations to these applications, so a calendar entry
-- carrying the same identifier is provably the same meeting.
--
-- Deleted mail still contributes its identifier. Deletion hides the message; it does not
-- un-schedule the meeting it announced, the same position the ledger takes on a deleted
-- reply.
SELECT a.id AS application_id,
       a.company_slug,
       a.role_title,
       coalesce(
           array_agg(em.ical_uid) FILTER (WHERE em.ical_uid IS NOT NULL AND em.ical_uid <> ''),
           '{}'
       )::text[] AS ical_uids
  FROM applications a
  LEFT JOIN emails em ON em.application_id = a.id AND em.user_id = a.user_id
 WHERE a.user_id = sqlc.arg(user_id)
 GROUP BY a.id, a.company_slug, a.role_title
 ORDER BY a.id;

-- name: ListApplicationInterviewsInRange :many
-- One caller's meetings over a date range, for the tracking calendar's second layer.
--
-- Cancelled meetings are returned and marked rather than filtered: the view shows them
-- struck through, because a silently missing interview reads as a fault.
SELECT i.id, i.application_id, i.ical_uid, i.starts_at, i.ends_at, i.title, i.join_url,
       i.status, a.company_slug, a.role_title, j.public_slug AS job_slug
  FROM application_interviews i
  JOIN applications a ON a.id = i.application_id
  LEFT JOIN jobs j    ON j.id = a.job_id
 WHERE i.user_id    = sqlc.arg(user_id)
   AND i.starts_at >= sqlc.arg(from_at)
   AND i.starts_at <= sqlc.arg(to_at)
 ORDER BY i.starts_at, i.id;

-- name: UpsertCalendarGrant :exec
-- Store the grant a calendar consent produced, and record that it now covers the calendar.
--
-- Three things this deliberately does not do. It does not touch `email`: a candidate who
-- already connected a mailbox keeps its address, and one who granted only the calendar has
-- none to record — the calendar flow never reads the Gmail profile, because that needs the
-- mail scope they may not have given.
--
-- It does not replace the scope list, it unions with it. The consent is incremental and
-- the returned token covers everything granted so far, so overwriting would forget the
-- mail scope and stop the mail sync at the moment the candidate added their calendar.
--
-- And it does not distinguish insert from update, because both mean the same thing here:
-- this person has granted us their calendar.
INSERT INTO gmail_connections (user_id, email, refresh_token_enc, status, scopes)
VALUES (sqlc.arg(user_id), '', sqlc.arg(refresh_token_enc), 'connected', sqlc.arg(scopes)::text[])
ON CONFLICT (user_id) DO UPDATE
SET refresh_token_enc = EXCLUDED.refresh_token_enc,
    status            = 'connected',
    scopes            = ARRAY(SELECT DISTINCT unnest(gmail_connections.scopes || EXCLUDED.scopes));

-- name: RecordGrantScopes :exec
-- Note the scopes a grant carries without touching anything else, unioned for the same
-- reason as above. The Gmail connect calls this so a mailbox connected before the calendar
-- existed still records what it holds.
UPDATE gmail_connections
SET scopes = ARRAY(SELECT DISTINCT unnest(scopes || sqlc.arg(scopes)::text[]))
WHERE user_id = sqlc.arg(user_id);

-- name: ListCalendarConnections :many
-- The candidates whose grant actually covers the calendar. The scope check belongs in the
-- query rather than in the worker's loop: a connection that cannot answer is not a
-- connection to retry, and calling the API to find that out costs a quota unit per user
-- per run for an answer we already hold.
SELECT user_id, email, refresh_token_enc
  FROM gmail_connections
 WHERE status = 'connected'
   AND sqlc.arg(calendar_scope)::text = ANY (scopes)
 ORDER BY user_id;
