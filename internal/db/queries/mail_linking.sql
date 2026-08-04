-- name: GetUserApplication :one
-- The caller's interaction row for one job (the application-detail header).
-- last_activity_at and has_pending_suggestion mirror ListUserJobs deliberately: the follow-up gate
-- must reach the same silence verdict as the badge on the board, and two derivations of one rule
-- drift. See the column comment in 0059 for why followed_up_at is NOT part of the activity.
SELECT uj.viewed_at, uj.saved_at, a.applied_at, a.stage, a.notes, a.followed_up_at,
       (CASE WHEN a.applied_at IS NOT NULL THEN
          GREATEST(a.applied_at,
                   (SELECT max(e.received_at)
                      FROM emails e
                     WHERE e.user_id = uj.user_id
                       AND e.job_id = uj.job_id
                       AND e.deleted_at IS NULL))
        END)::timestamptz AS last_activity_at,
       (a.applied_at IS NOT NULL AND EXISTS (
          SELECT 1
            FROM emails e
           WHERE e.user_id = uj.user_id
             AND e.suggested_job_id = uj.job_id
             -- "Pending" means the caller has not confirmed it, and confirming is
             -- what attaches the application — the same test the inbox's link
             -- filter makes, so the two cannot disagree about one message.
             AND e.application_id IS NULL
             AND e.deleted_at IS NULL))::boolean AS has_pending_suggestion
FROM user_jobs uj
LEFT JOIN applications a ON a.user_id = uj.user_id AND a.job_id = uj.job_id
WHERE uj.user_id = $1 AND uj.job_id = $2;

-- name: ListJobEmails :many
-- The emails linked to one of the caller's applications, newest first, for the
-- application detail page.
SELECT id, source, from_addr, from_name, subject, status_signal, link_source,
    received_at, (read_at IS NOT NULL)::boolean AS read
FROM emails
WHERE user_id = $1 AND job_id = $2
ORDER BY received_at DESC, id DESC;

-- name: ConfirmEmailLink :execrows
-- Promote a suggested link to a confirmed one: the suggestion becomes job_id with
-- link_source 'manual'. No-op (0 rows) when there is no pending suggestion.
UPDATE emails
SET job_id           = suggested_job_id,
    application_id   = (SELECT a.id FROM applications a
                         WHERE a.user_id = emails.user_id AND a.job_id = emails.suggested_job_id),
    link_source      = 'manual',
    suggested_job_id = NULL
WHERE emails.id = $1 AND emails.user_id = $2 AND emails.suggested_job_id IS NOT NULL;

-- name: RejectEmailLink :execrows
-- Dismiss a suggestion without linking.
UPDATE emails
SET suggested_job_id = NULL
WHERE id = $1 AND user_id = $2 AND suggested_job_id IS NOT NULL;

-- name: LinkEmailToJob :execrows
-- Manually link (or relink) an email to a chosen application, overriding any
-- auto-link or suggestion.
UPDATE emails
SET job_id           = $3,
    application_id   = (SELECT a.id FROM applications a
                         WHERE a.user_id = emails.user_id AND a.job_id = $3),
    link_source      = 'manual',
    suggested_job_id = NULL
WHERE emails.id = $1 AND emails.user_id = $2;

-- name: UnlinkEmail :execrows
-- Clear an email's application link (leaves the classified status intact).
UPDATE emails
SET job_id         = NULL,
    application_id = NULL,
    link_source = NULL
WHERE id = $1 AND user_id = $2;

-- name: ListEmailsForRecall :many
-- The net for the pull direction: from an application, the caller's mail that might
-- belong to it. Mail attached to nothing, inside a window around the application's
-- recorded date, OLDEST first, bounded.
--
-- The order and the closing edge are one decision with the cap, not three. Newest-first
-- over an open-ended window spends the forty candidates on a busy mailbox's most recent
-- mail, so a three-month-old application never shows the model the acknowledgement that
-- proves it — the button answers "nothing found" on exactly the applications people press
-- it for. Oldest-first inside a closed window makes the cap trim the far tail instead.
--
-- It filters on attachment state and time and NOT on the employer's name, which is the
-- one thing a reader expects to find here. Two measurements say not to. The name is
-- absent from the message body in 16 of 99 confirmed-correct links on a live mailbox —
-- recruiters routinely write without naming the employer — and body_text is EMPTY for
-- HTML-only senders (Gem, Ashby, Greenhouse), so an ILIKE over it is blind exactly where
-- the recruiting mail is. The narrowing is the caller's to do with the readable body.
--
-- Unattached means BOTH columns are null, and it takes both to say it. A message can
-- hold job_id with application_id still NULL: the matcher is offered saved-only jobs
-- (ListUserApplicationsForMatch admits uj.saved_at), SetEmailClassification then derives
-- an application that does not exist yet and leaves it NULL, and MarkJobApplied never
-- goes back to repair the mail — only cmd/backfill-applications does, and it is a
-- one-shot. Testing application_id alone would let that message into the net and end in a
-- confirm that RE-LINKS it, which this change declared out of scope.
--
-- What remains admitted is the mail nothing has claimed and the mail carrying an
-- unconfirmed suggestion — the second being the very case the button exists to fix.
--
-- A query of its own rather than new parameters on ListEmails, which serves the web
-- inbox and seven assistant tools: one shared statement grown for one reader is how the
-- two drift.
SELECT id, from_addr, from_name, subject, body_text, body_html, received_at, ical_uid
FROM emails
WHERE user_id = $1
  AND deleted_at IS NULL
  AND job_id IS NULL
  AND application_id IS NULL
  AND received_at >= sqlc.arg(since)
  AND received_at < sqlc.arg(until)
ORDER BY received_at ASC, id ASC
LIMIT sqlc.arg(lim);

-- name: SuggestJobForEmail :execrows
-- Record one message as belonging to a job the caller named, as a SUGGESTION they still
-- confirm. It is the only write the recall path makes.
--
-- It names a JOB, like LinkEmailToJob and like the column it writes: the application is
-- what ConfirmEmailLink derives when the caller accepts.
--
-- The two IS NULL predicates are the guard, not an optimisation: a linked message stays
-- unreachable from here even if the net, the model and the service layer went wrong at
-- once. Keep them in the statement — a check in Go is a check the next caller can skip —
-- and keep BOTH, for the reason ListEmailsForRecall spells out above. Clobbering
-- match_confidence is the concrete harm: it belongs to the LINK, so writing it here
-- would restate how sure somebody was about a link this statement did not make.
--
-- The cast is load-bearing. suggested_job_id is nullable, so sqlc.arg alone yields a
-- nullable parameter, and a zero-value one would CLEAR the suggestion while reporting a
-- row changed. This statement never clears — that is RejectEmailLink — so the mistake is
-- made unrepresentable rather than documented.
--
-- An unconfirmed suggestion naming a different job is overwritten. The caller asked
-- about this application explicitly, suggested_job_id holds one value, and a proposal
-- nobody has confirmed costs nothing to lose.
UPDATE emails
SET suggested_job_id = sqlc.arg(suggested_job_id)::bigint,
    match_confidence = sqlc.arg(confidence)::real
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id)
  AND job_id IS NULL AND application_id IS NULL AND deleted_at IS NULL;

-- name: GetEmailIDByExternalID :one
-- One message's id from the identifier its provider gave it, scoped to the caller.
--
-- The recall sweep proposes messages by PROVIDER id, because a searched message is not ours
-- until somebody links it. This is the one lookup that turns the id a caller pressed into
-- the row every linking path works on, immediately after the import stored it.
SELECT id FROM emails WHERE user_id = $1 AND external_id = $2 AND deleted_at IS NULL;
