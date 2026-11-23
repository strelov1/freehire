-- name: GetInterviewSchedulingSettings :one
SELECT u.interview_scheduling_enabled_at, u.interview_calendar_invites_enabled,
       coalesce(gc.status = 'connected' AND sqlc.arg(required_scopes)::text[] <@ gc.scopes, false)::boolean AS grant_ready
FROM users u
LEFT JOIN gmail_connections gc ON gc.user_id = u.id
WHERE u.id = sqlc.arg(user_id);

-- name: EnableInterviewScheduling :exec
UPDATE users SET interview_scheduling_enabled_at = coalesce(interview_scheduling_enabled_at, now()),
                 interview_calendar_invites_enabled = true
WHERE id = $1;

-- name: DisableInterviewScheduling :exec
UPDATE users SET interview_scheduling_enabled_at = NULL,
                 interview_calendar_invites_enabled = false
WHERE id = $1;

-- name: SeedInterviewSchedulingThreads :exec
INSERT INTO interview_scheduling_threads (user_id, application_id, anchor_email_id, gmail_thread_id, recruiter_email)
SELECT DISTINCT ON (e.user_id, e.thread_id) e.user_id, e.application_id, e.id, e.thread_id, e.from_addr
FROM emails e
JOIN users u ON u.id = e.user_id
JOIN applications a ON a.id = e.application_id AND a.user_id = e.user_id
JOIN gmail_connections gc ON gc.user_id = e.user_id
LEFT JOIN interview_scheduling_threads s ON s.user_id = e.user_id AND s.gmail_thread_id = e.thread_id
WHERE e.source = 'gmail' AND e.direction = 'incoming' AND e.deleted_at IS NULL
  AND e.status_signal = 'interview_invitation' AND e.link_source = 'auto'
  AND e.thread_id <> '' AND e.from_addr <> '' AND coalesce(e.ical_uid, '') = ''
  AND e.received_at > greatest(u.interview_scheduling_enabled_at, now() - interval '7 days')
  AND u.interview_scheduling_enabled_at IS NOT NULL AND u.interview_calendar_invites_enabled
  AND u.banned_at IS NULL AND greatest(u.pro_until, u.ultra_until) > now()
  AND coalesce(a.stage, 'applied') NOT IN ('accepted', 'rejected', 'withdrawn')
  AND gc.status = 'connected' AND gc.email <> '' AND lower(gc.email) <> lower(e.from_addr)
  AND sqlc.arg(required_scopes)::text[] <@ gc.scopes
  AND (cardinality(sqlc.arg(allow_users)::bigint[]) = 0 OR e.user_id = ANY(sqlc.arg(allow_users)::bigint[]))
  AND s.id IS NULL
ORDER BY e.user_id, e.thread_id, e.received_at DESC, e.id DESC
LIMIT 200
ON CONFLICT (user_id, gmail_thread_id) DO NOTHING;

-- name: ClaimInterviewSchedulingThread :one
WITH due AS (
    SELECT s.id, gc.email AS mailbox_email, gc.refresh_token_enc,
           coalesce(nullif(u.timezone, ''), 'UTC')::text AS candidate_zone,
           e.external_id AS anchor_provider_id
    FROM interview_scheduling_threads s
    JOIN users u ON u.id = s.user_id
    JOIN gmail_connections gc ON gc.user_id = s.user_id
    JOIN applications a ON a.id = s.application_id
    JOIN emails e ON e.id = s.anchor_email_id
    WHERE s.status IN ('new', 'waiting', 'calendar') AND s.next_at <= now()
      AND u.interview_scheduling_enabled_at IS NOT NULL AND u.interview_calendar_invites_enabled
      AND e.received_at > u.interview_scheduling_enabled_at AND e.deleted_at IS NULL
      AND e.application_id = s.application_id AND e.link_source = 'auto'
      AND u.banned_at IS NULL AND greatest(u.pro_until, u.ultra_until) > now()
      AND coalesce(a.stage, 'applied') NOT IN ('accepted', 'rejected', 'withdrawn')
      AND gc.status = 'connected' AND gc.email <> '' AND sqlc.arg(required_scopes)::text[] <@ gc.scopes
      AND (cardinality(sqlc.arg(allow_users)::bigint[]) = 0 OR s.user_id = ANY(sqlc.arg(allow_users)::bigint[]))
    ORDER BY s.next_at, s.id
    LIMIT 1
    FOR UPDATE OF s SKIP LOCKED
)
UPDATE interview_scheduling_threads s
SET next_at = now() + interval '4 minutes'
FROM due
WHERE s.id = due.id
RETURNING s.*, due.mailbox_email, due.refresh_token_enc, due.candidate_zone, due.anchor_provider_id;

-- name: ScheduleInterviewReply :exec
UPDATE interview_scheduling_threads
SET pending_kind = sqlc.arg(kind), pending_body = sqlc.arg(body),
    pending_parent_id = sqlc.arg(parent_id), pending_message_id = sqlc.arg(message_id),
    last_processed_id = sqlc.arg(processed_id), offered_starts = sqlc.arg(offered_starts),
    offer_zone = sqlc.arg(offer_zone), language = sqlc.arg(language), attempted_at = NULL,
    next_at = now()
WHERE interview_scheduling_threads.id = sqlc.arg(id) AND pending_kind = '';

-- name: BeginInterviewReply :execrows
UPDATE interview_scheduling_threads s
SET attempted_at = now(), next_at = now() + interval '2 minutes'
FROM users u, gmail_connections gc
WHERE s.id = sqlc.arg(id) AND s.user_id = u.id AND u.id = gc.user_id
  AND s.pending_kind <> '' AND s.attempted_at IS NULL
  AND u.interview_scheduling_enabled_at IS NOT NULL AND u.interview_calendar_invites_enabled
  AND u.banned_at IS NULL AND greatest(u.pro_until, u.ultra_until) > now()
  AND gc.status = 'connected' AND sqlc.arg(required_scopes)::text[] <@ gc.scopes
  AND gc.refresh_token_enc = sqlc.arg(refresh_token_enc);

-- name: FinishInterviewReply :exec
WITH finished AS (
    UPDATE interview_scheduling_threads
    SET status = CASE WHEN pending_kind = 'confirm' THEN 'booked' ELSE 'waiting' END,
        followups = CASE pending_kind WHEN 'offer' THEN 0 WHEN 'followup' THEN followups + 1 ELSE followups END,
        followup_at = CASE
            WHEN pending_kind = 'confirm' THEN NULL
            WHEN pending_kind = 'followup' AND followups = 1 THEN now() + interval '1 minute'
            ELSE now() + interval '2 days' END,
        last_sent_id = sqlc.arg(sent_id), next_at = now() + interval '1 minute',
        pending_kind = '', pending_body = '', pending_parent_id = '',
        pending_message_id = '', attempted_at = NULL
    WHERE interview_scheduling_threads.id = sqlc.arg(id) AND pending_message_id = sqlc.arg(message_id)
    RETURNING id, user_id, status
)
INSERT INTO interview_scheduling_activity (user_id, thread_id, kind)
SELECT user_id, id, CASE WHEN status = 'booked' THEN 'booking' ELSE 'reply' END
FROM finished;

-- name: SelectInterviewTime :exec
UPDATE interview_scheduling_threads
SET status = 'calendar', selected_start = sqlc.arg(starts_at), last_processed_id = sqlc.arg(message_id),
    language = sqlc.arg(language), offer_zone = sqlc.arg(offer_zone), next_at = now()
WHERE interview_scheduling_threads.id = sqlc.arg(id) AND status IN ('new', 'waiting');

-- name: GetSchedulingBooking :one
SELECT b.calendar_invited_at, b.calendar_event_key
FROM booking_link_bookings b
JOIN booking_links l ON l.id = b.link_id
WHERE l.user_id = sqlc.arg(user_id) AND l.application_id = sqlc.arg(application_id)
  AND b.starts_at = sqlc.arg(starts_at) AND lower(b.recruiter_email) = lower(sqlc.arg(recruiter_email)::text)
  AND b.status = 'confirmed';

-- name: EndInterviewScheduling :exec
WITH finished AS (
    UPDATE interview_scheduling_threads
    SET status = sqlc.arg(status), pending_kind = '', pending_body = '',
        pending_parent_id = '', pending_message_id = '', attempted_at = NULL
    WHERE interview_scheduling_threads.id = sqlc.arg(id) AND status IN ('new', 'waiting', 'calendar')
    RETURNING id, user_id, status
)
INSERT INTO interview_scheduling_activity (user_id, thread_id, kind)
SELECT user_id, id, 'review' FROM finished WHERE status = 'review';

-- name: FinishSchedulingLinkBooking :execrows
-- A recruiter may take the fallback link instead of replying in Gmail.
WITH finished AS (
    UPDATE interview_scheduling_threads s
    SET status = 'booked', followup_at = NULL, pending_kind = '', pending_body = '',
        pending_parent_id = '', pending_message_id = '', attempted_at = NULL
    WHERE s.id = sqlc.arg(id) AND s.status IN ('new', 'waiting')
      AND EXISTS (
        SELECT 1 FROM booking_link_bookings b
        JOIN booking_links l ON l.id = b.link_id
        JOIN emails e ON e.id = s.anchor_email_id
        WHERE l.application_id = s.application_id AND b.user_id = s.user_id
          AND lower(b.recruiter_email) = lower(s.recruiter_email)
          AND b.created_at >= e.received_at AND b.status = 'confirmed'
      )
    RETURNING s.user_id, s.id
)
INSERT INTO interview_scheduling_activity (user_id, thread_id, kind)
SELECT user_id, id, 'booking' FROM finished;
