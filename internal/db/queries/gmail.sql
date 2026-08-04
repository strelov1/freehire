-- name: GetGmailConnection :one
-- The grant row as the status endpoint reads it. `scopes` is included because the two
-- consents are separate: a connected mailbox says nothing about the calendar, and a
-- calendar grant may have no mailbox behind it, so the row's existence cannot answer
-- either question on its own.
SELECT user_id, email, status, sync_cursor, connected_at, last_synced_at, scopes
FROM gmail_connections
WHERE user_id = $1;

-- name: GetGmailRefreshToken :one
SELECT refresh_token_enc, status, sync_cursor
FROM gmail_connections
WHERE user_id = $1;

-- name: UpsertGmailConnection :exec
-- Connect (or reconnect) a user's Gmail: store the encrypted refresh token and
-- mark connected, preserving the sync cursor on reconnect.
INSERT INTO gmail_connections (user_id, email, refresh_token_enc, status)
VALUES ($1, $2, $3, 'connected')
ON CONFLICT (user_id) DO UPDATE
SET email = EXCLUDED.email,
    refresh_token_enc = EXCLUDED.refresh_token_enc,
    status = 'connected';

-- name: ListConnectedGmailUsers :many
-- Drives the sync worker: every connection still authorized AND holding a mailbox.
--
-- The address is the test, and it is not decoration. Since the calendar consent exists,
-- a row here may be a Google grant with no mail scope at all — UpsertCalendarGrant
-- inserts one with an empty address and never fills it. Syncing such a user calls the
-- Gmail API with a token that cannot answer, takes the 403 as a revoked grant, and flips
-- the SHARED status to needs_reconsent — killing the calendar sync they actually asked
-- for, one cron tick after they connected it.
--
-- Checked on the address rather than on `scopes` for two reasons: every row that predates
-- the scopes column has an empty list and would be excluded by a scope test, and the
-- scope string then has to be spelled in SQL as well as in Go.
SELECT user_id, email, sync_cursor
FROM gmail_connections
WHERE status = 'connected' AND email <> '';

-- name: SetGmailSynced :exec
UPDATE gmail_connections
SET sync_cursor = $2, last_synced_at = now()
WHERE user_id = $1;

-- name: SetGmailStatus :exec
UPDATE gmail_connections SET status = $2 WHERE user_id = $1;

-- name: DeleteGmailConnection :exec
DELETE FROM gmail_connections WHERE user_id = $1;

-- name: DeleteEmailsBySource :exec
-- Purge one source's mail for a user (Gmail disconnect passes 'gmail', mailbox
-- release passes 'hosted') — the other source's mail is left untouched.
DELETE FROM emails WHERE user_id = $1 AND source = $2;

-- name: UpsertEmail :exec
-- Store a Gmail message, idempotent by (user_id, source, external_id) with
-- source fixed to 'gmail'; the hosted path has its own insert (InsertHostedMessage).
INSERT INTO emails (
    user_id, source, external_id, thread_id, from_addr, from_name,
    subject, body_text, body_html, received_at, ical_uid
) VALUES ($1, 'gmail', $2, $3, $4, $5, $6, $7, $8, $9, sqlc.arg(ical_uid))
ON CONFLICT (user_id, source, external_id) DO NOTHING;

-- name: UpsertExternalEmail :one
-- Store a message the caller's own harness fetched, under source 'external' and
-- idempotent by (user_id, source, external_id) so a re-sync updates rather than
-- duplicates. `inserted` distinguishes a first push from a re-push (xmax is 0 only
-- on a genuine insert), so the ingest endpoint can report both counts.
--
-- The conflict branch refreshes ONLY the content columns. read_at, deleted_at and
-- every classification column are the reader's state, not the mail server's: a
-- nightly re-sync must not un-read a message, resurrect a deleted one, or wipe the
-- agent's triage verdict.
INSERT INTO emails (
    user_id, source, external_id, thread_id, from_addr, from_name,
    subject, body_text, body_html, received_at, ical_uid
) VALUES ($1, 'external', $2, $3, $4, $5, $6, $7, $8, $9, sqlc.arg(ical_uid))
ON CONFLICT (user_id, source, external_id) DO UPDATE
SET thread_id   = EXCLUDED.thread_id,
    from_addr   = EXCLUDED.from_addr,
    from_name   = EXCLUDED.from_name,
    subject     = EXCLUDED.subject,
    body_text   = EXCLUDED.body_text,
    body_html   = EXCLUDED.body_html,
    received_at = EXCLUDED.received_at,
    -- A content column like the rest: the meeting identifier belongs to the message,
    -- not to the reader, so a re-sync may refresh it. The reader's own state — read_at,
    -- deleted_at, every classification column — still stays out of this list.
    ical_uid    = EXCLUDED.ical_uid
RETURNING id, (xmax = 0)::boolean AS inserted;

-- name: ListEmails :many
-- Flat inbox listing, newest first — one row per message (no subject grouping),
-- soft-deleted messages excluded. Optional filters (each empty/false = no filter):
-- source narrows to one account; unread hides already-read mail; status narrows to
-- one classified signal; unclassified narrows to mail awaiting triage (the agent's
-- work queue, since 'external' mail is never enqueued for the worker); link
-- narrows to one link state; the search term matches subject, sender, or body.
-- The snippet is the body's leading text with whitespace collapsed, for the list
-- row.
--
-- The three link states partition the mailbox — 'linked' is attached to an
-- application, 'suggested' has a pending suggestion and no link, 'unlinked' has
-- neither — so their counts always sum to the unfiltered total. A message that is
-- both linked and carrying a stale suggestion reads as linked: the resolved
-- answer wins over the proposal it superseded.
-- The link/classification columns ride alongside so the inbox can render the
-- confirm chip and application link without a second lookup; the LEFT JOINs
-- resolve the linked/suggested application's public slug + company for display.
--
-- with_body sends the full bodies too, for an agent that classifies a whole page
-- without a GetEmail per message — which would also mark each one read. The
-- snippet already detoasts body_text, so the extra column costs no extra read;
-- it is guarded only to keep the web inbox's payload small.
SELECT emails.id, emails.source, emails.external_id, emails.from_addr, emails.from_name, emails.subject,
    left(regexp_replace(emails.body_text, E'\\s+', ' ', 'g'), 160)::text AS snippet,
    (CASE WHEN sqlc.arg(with_body)::bool THEN emails.body_text ELSE '' END)::text AS body_text,
    (CASE WHEN sqlc.arg(with_body)::bool THEN emails.body_html ELSE '' END)::text AS body_html,
    emails.received_at, (emails.read_at IS NOT NULL)::boolean AS read,
    emails.job_id, emails.suggested_job_id, emails.status_signal, emails.link_source,
    -- The link is to the APPLICATION; the posting is how it is displayed while the
    -- catalogue still holds one. cmd/prune clears la.job_id, so linked_slug goes NULL
    -- while the row stays linked — the employer is on the application for exactly that.
    lj.public_slug AS linked_slug,
    COALESCE(lj.company, la.company_slug, '')::text AS linked_company,
    sj.public_slug AS suggested_slug, sj.company AS suggested_company
FROM emails
LEFT JOIN applications la ON la.id = emails.application_id
LEFT JOIN jobs lj ON lj.id = la.job_id
LEFT JOIN jobs sj ON sj.id = emails.suggested_job_id
WHERE emails.user_id = $1
  AND emails.deleted_at IS NULL
  AND (sqlc.arg(src)::text = '' OR emails.source = sqlc.arg(src))
  AND (sqlc.arg(unread)::bool = false OR emails.read_at IS NULL)
  AND (sqlc.arg(status)::text = '' OR emails.status_signal = sqlc.arg(status))
  AND (sqlc.arg(unclassified)::bool = false OR emails.classified_at IS NULL)
  AND (
    sqlc.arg(link)::text = ''
    OR (sqlc.arg(link) = 'linked'    AND emails.application_id IS NOT NULL)
    OR (sqlc.arg(link) = 'suggested' AND emails.application_id IS NULL AND emails.suggested_job_id IS NOT NULL)
    OR (sqlc.arg(link) = 'unlinked'  AND emails.application_id IS NULL AND emails.suggested_job_id IS NULL)
  )
  AND (
    sqlc.arg(q)::text = ''
    OR emails.subject   ILIKE '%' || sqlc.arg(q) || '%'
    OR emails.from_name ILIKE '%' || sqlc.arg(q) || '%'
    OR emails.from_addr ILIKE '%' || sqlc.arg(q) || '%'
    OR emails.body_text ILIKE '%' || sqlc.arg(q) || '%'
  )
  -- The inbox's default: mail the classifier judged not to be about an application at
  -- all is omitted. The judgement is `mailclassify`'s, on a call already made, rather
  -- than a curated list of senders — a list would be a second judge, maintained by hand
  -- forever against people whose business is registering domains, and it would judge by
  -- sender where the classifier judges by content.
  --
  -- Unclassified mail is NEVER hidden. A message nothing has judged has not been found
  -- irrelevant; it has not been looked at.
  AND (sqlc.arg(include_other)::bool OR coalesce(emails.status_signal, '') <> 'other')
ORDER BY emails.received_at DESC, emails.id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountEmails :one
-- Total live messages for the caller under the same optional filters as ListEmails, plus
-- how many of them the `other` default omitted.
--
-- Both numbers come from one statement and one set of predicates on purpose: a hidden count
-- computed separately would describe a different mailbox from the one on screen the moment
-- any filter is active. The count is not decoration — a filter that hides silently makes a
-- misclassification impossible to find, and the classifier reads attacker-controlled text.
SELECT
    count(*) FILTER (WHERE sqlc.arg(include_other)::bool OR coalesce(status_signal, '') <> 'other')::bigint AS total,
    count(*) FILTER (WHERE NOT sqlc.arg(include_other)::bool AND coalesce(status_signal, '') = 'other')::bigint AS hidden
FROM emails
WHERE user_id = $1
  AND deleted_at IS NULL
  AND (sqlc.arg(src)::text = '' OR source = sqlc.arg(src))
  AND (sqlc.arg(unread)::bool = false OR read_at IS NULL)
  AND (sqlc.arg(status)::text = '' OR status_signal = sqlc.arg(status))
  AND (sqlc.arg(unclassified)::bool = false OR classified_at IS NULL)
  AND (
    sqlc.arg(link)::text = ''
    OR (sqlc.arg(link) = 'linked'    AND application_id IS NOT NULL)
    OR (sqlc.arg(link) = 'suggested' AND application_id IS NULL AND suggested_job_id IS NOT NULL)
    OR (sqlc.arg(link) = 'unlinked'  AND application_id IS NULL AND suggested_job_id IS NULL)
  )
  AND (
    sqlc.arg(q)::text = ''
    OR subject   ILIKE '%' || sqlc.arg(q) || '%'
    OR from_name ILIKE '%' || sqlc.arg(q) || '%'
    OR from_addr ILIKE '%' || sqlc.arg(q) || '%'
    OR body_text ILIKE '%' || sqlc.arg(q) || '%'
  );

-- name: GetInterviewInvitation :one
-- The employer's own description of an upcoming interview, for the rehearsal context:
-- the most recent message classified as an invitation and linked to this application.
--
-- It is a query rather than a tool over ListEmails for two reasons. ListEmails cannot
-- express "for this vacancy" — it filters by link STATE, not by which job — and
-- filtering its page in Go would break pagination. And unlike GetEmail, this reads
-- without marking: read_at means a human opened the message, so an agent that consumed
-- it here would zero its owner's unread count.
--
-- Both bodies are returned, not just the text one: ATS senders routinely mail HTML
-- with no text/plain part, so body_text is empty exactly where an invitation is most
-- likely to come from. The caller renders one down to the other.
--
-- Only a CONFIRMED link counts: job_id is set by the deterministic matcher, while a
-- model's confident guess waits in suggested_job_id for the candidate to accept. An
-- unconfirmed guess putting another employer's interview into the rehearsal's context
-- is worse than the rehearsal having no invitation at all.
--
-- The signal is spelled here and as mailclassify.SignalInterviewInvitation in Go;
-- renaming the vocabulary term means changing both, and this side fails by matching
-- nothing rather than by failing to compile.
--
-- Owner-scoped like every other mail query, and soft-deleted mail is invisible.
SELECT id, from_addr, from_name, subject, body_text, body_html, received_at,
    (read_at IS NOT NULL)::boolean AS read
FROM emails
WHERE user_id = $1
  AND job_id = sqlc.arg(job_id)::bigint
  AND status_signal = 'interview_invitation'
  AND deleted_at IS NULL
ORDER BY received_at DESC, id DESC
LIMIT 1;

-- name: GetEmail :one
SELECT emails.id, emails.source, emails.external_id, emails.s3_key, emails.from_addr, emails.from_name, emails.subject,
    emails.body_text, emails.body_html, emails.received_at, (emails.read_at IS NOT NULL)::boolean AS read,
    emails.job_id, emails.suggested_job_id, emails.status_signal, emails.link_source,
    -- The link is to the APPLICATION; the posting is how it is displayed while the
    -- catalogue still holds one. cmd/prune clears la.job_id, so linked_slug goes NULL
    -- while the row stays linked — the employer is on the application for exactly that.
    lj.public_slug AS linked_slug,
    COALESCE(lj.company, la.company_slug, '')::text AS linked_company,
    sj.public_slug AS suggested_slug, sj.company AS suggested_company
FROM emails
LEFT JOIN applications la ON la.id = emails.application_id
LEFT JOIN jobs lj ON lj.id = la.job_id
LEFT JOIN jobs sj ON sj.id = emails.suggested_job_id
WHERE emails.id = $1 AND emails.user_id = $2 AND emails.deleted_at IS NULL;

-- name: MarkEmailRead :exec
-- Stamp read on first open; a no-op once already read.
UPDATE emails SET read_at = now()
WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkAllEmailsRead :execrows
-- Bulk mark-as-read for the caller, honoring the same optional filters as the
-- listing, so "mark all read" means "everything currently shown". Only unread,
-- live rows are touched; returns how many it marked.
UPDATE emails SET read_at = now()
WHERE user_id = $1
  AND read_at IS NULL
  AND deleted_at IS NULL
  AND (sqlc.arg(src)::text = '' OR source = sqlc.arg(src))
  AND (sqlc.arg(status)::text = '' OR status_signal = sqlc.arg(status))
  AND (
    sqlc.arg(link)::text = ''
    OR (sqlc.arg(link) = 'linked'    AND application_id IS NOT NULL)
    OR (sqlc.arg(link) = 'suggested' AND application_id IS NULL AND suggested_job_id IS NOT NULL)
    OR (sqlc.arg(link) = 'unlinked'  AND application_id IS NULL AND suggested_job_id IS NULL)
  )
  AND (
    sqlc.arg(q)::text = ''
    OR subject   ILIKE '%' || sqlc.arg(q) || '%'
    OR from_name ILIKE '%' || sqlc.arg(q) || '%'
    OR from_addr ILIKE '%' || sqlc.arg(q) || '%'
    OR body_text ILIKE '%' || sqlc.arg(q) || '%'
  );

-- name: SoftDeleteEmail :execrows
-- Soft-delete one message (hidden from the listing, retained for restore),
-- scoped to the caller and idempotent. Returns 0 rows only when it is not the
-- caller's message (→ 404).
UPDATE emails SET deleted_at = now()
WHERE id = $1 AND user_id = $2;

-- name: RestoreEmail :execrows
-- Undo a soft-delete, scoped to the caller and idempotent. Returns 0 rows only
-- when it is not the caller's message (→ 404).
UPDATE emails SET deleted_at = NULL
WHERE id = $1 AND user_id = $2;

-- name: CountEmailsByState :many
-- The mailbox's shape in one pass: one row per classification label (the empty
-- label being mail nothing has judged yet), carrying that label's total plus how
-- many of it are unread, unclassified, linked to an application, or carrying a
-- pending suggestion. The caller sums the rows for the mailbox-wide totals — the
-- alternative, a FILTER column per label, would restate mailclassify's vocabulary
-- in SQL, where it would silently fall behind the Go one.
--
-- Soft-deleted mail is excluded, so these counts and the listing's agree.
SELECT coalesce(status_signal, '')::text AS label,
    count(*)::bigint AS n,
    count(*) FILTER (WHERE read_at IS NULL)::bigint AS unread,
    count(*) FILTER (WHERE classified_at IS NULL)::bigint AS unclassified,
    count(*) FILTER (WHERE application_id IS NOT NULL)::bigint AS linked,
    count(*) FILTER (WHERE application_id IS NULL AND suggested_job_id IS NOT NULL)::bigint AS suggested
FROM emails
WHERE user_id = $1 AND deleted_at IS NULL
GROUP BY 1;
