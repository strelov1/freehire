-- The messaging core (internal/engage/messaging). Every conversation today has exactly two
-- participants; queries that name "the other participant" rely on it, and a multi-party
-- kind will have to revisit them. Each participant speaks as a persona; anything that
-- names an account to another reads the persona, never the account.

-- name: StartConversation :one
-- A pending conversation, both participants and the starter's first message in ONE
-- statement, so a conversation never exists without the message that opened it or
-- without either side. A second conversation of the kind for the same pair_key is the
-- unique violation on (kind, pair_key), which the caller turns into a reopen or a
-- conflict. A block between the two in either direction is checked HERE and not in an
-- earlier read, so a block committing in between cannot slip a conversation through; no
-- row then means the pair is blocked.
WITH c AS (
    INSERT INTO conversations (kind, pair_key, starter_id)
    SELECT sqlc.arg(kind)::text, sqlc.narg(pair_key)::text, sqlc.arg(starter_id)::bigint
    WHERE NOT EXISTS (
        SELECT 1 FROM user_blocks
        WHERE (blocker_id = sqlc.arg(starter_id) AND blocked_id = sqlc.arg(recipient_id))
           OR (blocker_id = sqlc.arg(recipient_id) AND blocked_id = sqlc.arg(starter_id)))
    RETURNING *
), p AS (
    INSERT INTO conversation_participants (conversation_id, user_id, persona)
    SELECT c.id, u.user_id, u.persona FROM c
    CROSS JOIN (VALUES (sqlc.arg(starter_id)::bigint, sqlc.arg(starter_persona)::text),
                       (sqlc.arg(recipient_id)::bigint, sqlc.arg(recipient_persona)::text)) AS u (user_id, persona)
), m AS (
    INSERT INTO messages (conversation_id, sender_id, body)
    SELECT c.id, c.starter_id, sqlc.arg(body)::text FROM c
)
SELECT * FROM c;

-- name: ReopenConversation :one
-- A new start on a pair_key whose conversation was closed WITHOUT a block (a feature closed
-- it, e.g. a buddy leaving; a block never reopens). The row is reused rather than
-- replaced, so the history stays and the pair keeps exactly one conversation. It goes back
-- to pending under the new starter, created_at restarts so it counts against the kind's
-- start cap, both notice stamps clear, and the old messages count as read for both sides
-- so only the new first message is unread. The read cursors are taken from the snapshot
-- before the new message is inserted. No row means closed-without-block did not hold.
WITH c AS (
    UPDATE conversations c
    SET status = 'pending',
        starter_id = sqlc.arg(starter_id)::bigint,
        created_at = now(),
        accepted_at = NULL,
        closed_at = NULL
    WHERE c.kind = sqlc.arg(kind)::text
      AND c.pair_key = sqlc.arg(pair_key)::text
      AND c.status = 'closed'
      AND NOT EXISTS (
          SELECT 1 FROM user_blocks
          WHERE (blocker_id = sqlc.arg(starter_id) AND blocked_id = sqlc.arg(recipient_id))
             OR (blocker_id = sqlc.arg(recipient_id) AND blocked_id = sqlc.arg(starter_id)))
    RETURNING c.*
), r AS (
    UPDATE conversation_participants p
    SET notified_at = NULL,
        last_read_message_id = (SELECT coalesce(max(msg.id), 0) FROM messages msg WHERE msg.conversation_id = p.conversation_id)
    FROM c
    WHERE p.conversation_id = c.id
), m AS (
    INSERT INTO messages (conversation_id, sender_id, body)
    SELECT c.id, c.starter_id, sqlc.arg(body)::text FROM c
)
SELECT * FROM c;

-- name: GetConversationByPairKey :one
-- The kind's conversation for a pair_key, in any status.
SELECT * FROM conversations WHERE kind = $1 AND pair_key = $2;

-- name: GetConversationForUser :one
-- One conversation, only when user_id takes part in it: an outsider gets no row, exactly
-- as for a conversation that does not exist.
SELECT c.*
FROM conversations c
JOIN conversation_participants p ON p.conversation_id = c.id AND p.user_id = sqlc.arg(user_id)
WHERE c.id = sqlc.arg(id);

-- name: ListConversationParticipants :many
SELECT user_id, persona FROM conversation_participants WHERE conversation_id = $1 ORDER BY user_id;

-- name: CountConversationsStartedSince :one
-- How many conversations of the kind starter_id started after since, and when the oldest
-- of them was started (NULL when there are none): that instant plus the window is when a
-- slot frees up.
SELECT count(*)::bigint AS started, min(created_at)::timestamptz AS oldest
FROM conversations
WHERE starter_id = sqlc.arg(starter_id) AND kind = sqlc.arg(kind) AND created_at > sqlc.arg(since);

-- name: UsersBlocked :one
-- A block between the two accounts in either direction.
SELECT EXISTS (
    SELECT 1 FROM user_blocks
    WHERE (blocker_id = sqlc.arg(a) AND blocked_id = sqlc.arg(b))
       OR (blocker_id = sqlc.arg(b) AND blocked_id = sqlc.arg(a))
) AS blocked;

-- name: AnswerConversation :one
-- Accept or decline a pending conversation. Only a participant who did not start it
-- matches, and no block may stand between the participants (a block closes the
-- conversation, but one committing between the caller's read and this write would not be
-- seen otherwise); no row means the caller must look the conversation up to say why.
UPDATE conversations c
SET status = sqlc.arg(status)::text,
    accepted_at = CASE WHEN sqlc.arg(status)::text = 'open' THEN now() ELSE c.accepted_at END
WHERE c.id = sqlc.arg(id)
  AND c.status = 'pending'
  AND c.starter_id <> sqlc.arg(user_id)
  AND EXISTS (
      SELECT 1 FROM conversation_participants p
      WHERE p.conversation_id = c.id AND p.user_id = sqlc.arg(user_id))
  AND NOT EXISTS (
      SELECT 1
      FROM conversation_participants p1
      JOIN conversation_participants p2 ON p2.conversation_id = p1.conversation_id AND p2.user_id <> p1.user_id
      JOIN user_blocks b ON b.blocker_id = p1.user_id AND b.blocked_id = p2.user_id
      WHERE p1.conversation_id = c.id)
RETURNING c.*;

-- name: InsertMessage :one
-- A message into an OPEN conversation the sender takes part in, and the sender's read
-- cursor moved onto it, in one statement. The status is guarded here, under FOR SHARE,
-- and not by an earlier read: a close that commits while this waits makes the row stop
-- matching, so no message lands after it. No row means the caller must look the
-- conversation up to say why.
WITH m AS (
    INSERT INTO messages (conversation_id, sender_id, body)
    SELECT c.id, sqlc.arg(sender_id)::bigint, sqlc.arg(body)::text
    FROM conversations c
    JOIN conversation_participants p ON p.conversation_id = c.id AND p.user_id = sqlc.arg(sender_id)
    WHERE c.id = sqlc.arg(conversation_id)
      AND c.status = 'open'
    FOR SHARE OF c
    RETURNING id, conversation_id, sender_id, body, created_at
), r AS (
    UPDATE conversation_participants p
    SET last_read_message_id = m.id
    FROM m
    WHERE p.conversation_id = m.conversation_id AND p.user_id = m.sender_id
)
SELECT id, sender_id, body, created_at FROM m;

-- name: ListMessages :many
-- Messages after a given id, oldest first. Readability is the caller's check.
SELECT id, sender_id, body, created_at
FROM messages
WHERE conversation_id = sqlc.arg(conversation_id) AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(page_limit);

-- name: MarkConversationRead :one
-- Move the caller's read cursor up to up_to, never back and never past the conversation's
-- newest message. No row means the caller does not take part.
UPDATE conversation_participants p
SET last_read_message_id = GREATEST(coalesce(p.last_read_message_id, 0), LEAST(sqlc.arg(up_to)::bigint, newest.id))
FROM (
    SELECT coalesce(max(id), 0)::bigint AS id
    FROM messages WHERE conversation_id = sqlc.arg(conversation_id)
) newest
WHERE p.conversation_id = sqlc.arg(conversation_id)
  AND p.user_id = sqlc.arg(user_id)
RETURNING p.conversation_id;

-- name: ListConversations :many
-- Every conversation of the viewer with the other participant's id, the viewer's unread
-- count (the other side's messages past the viewer's read cursor) and the newest message
-- time, newest first. A declined conversation is hidden from the participant who declined
-- it. The unread count and last time come from one pass over each conversation's
-- messages, so the list costs one query however many conversations there are.
SELECT c.id,
       c.kind,
       c.starter_id,
       c.status,
       c.created_at,
       c.accepted_at,
       other.user_id AS other_id,
       other.persona AS other_persona,
       stats.unread::bigint AS unread,
       coalesce(stats.newest, c.created_at)::timestamptz AS last_message_at
FROM conversation_participants me
JOIN conversations c ON c.id = me.conversation_id
JOIN conversation_participants other ON other.conversation_id = c.id AND other.user_id <> me.user_id
CROSS JOIN LATERAL (
    SELECT count(*) FILTER (
               WHERE msg.sender_id <> me.user_id
                 AND msg.id > coalesce(me.last_read_message_id, 0)
           ) AS unread,
           max(msg.created_at) AS newest
    FROM messages msg
    WHERE msg.conversation_id = c.id
) stats
WHERE me.user_id = sqlc.arg(viewer_id)
  AND NOT (c.status = 'declined' AND c.starter_id <> sqlc.arg(viewer_id))
ORDER BY last_message_at DESC, c.id DESC;

-- name: BlockFromConversation :one
-- Block the other participant of a conversation and close EVERY conversation between the
-- two, of any kind, in one statement so no half can land alone. Only a participant
-- matches; no row means the caller must say the conversation is not theirs. Closing is
-- allowed from any status, and a conversation that was already closed keeps its original
-- closed_at. Blocking twice writes nothing the second time.
WITH me AS (
    SELECT p.conversation_id
    FROM conversation_participants p
    WHERE p.conversation_id = sqlc.arg(conversation_id) AND p.user_id = sqlc.arg(blocker_id)
), others AS (
    SELECT p.user_id
    FROM conversation_participants p
    JOIN me ON me.conversation_id = p.conversation_id
    WHERE p.user_id <> sqlc.arg(blocker_id)
), b AS (
    INSERT INTO user_blocks (blocker_id, blocked_id)
    SELECT sqlc.arg(blocker_id)::bigint, others.user_id FROM others
    ON CONFLICT DO NOTHING
), closed AS (
    UPDATE conversations c
    SET status = 'closed', closed_at = coalesce(c.closed_at, now())
    WHERE c.id IN (
        SELECT mine.conversation_id
        FROM conversation_participants mine
        JOIN conversation_participants theirs ON theirs.conversation_id = mine.conversation_id
        WHERE mine.user_id = sqlc.arg(blocker_id)
          AND theirs.user_id IN (SELECT user_id FROM others))
)
SELECT conversation_id FROM me;

-- name: InsertConversationReport :one
-- A report of a conversation by one of its participants. No row means either the reporter
-- does not take part or they already reported it; the caller looks the conversation up to
-- say which.
INSERT INTO conversation_reports (conversation_id, reporter_id, reason, note)
SELECT p.conversation_id, p.user_id, sqlc.arg(reason)::text, sqlc.narg(note)::text
FROM conversation_participants p
WHERE p.conversation_id = sqlc.arg(conversation_id)
  AND p.user_id = sqlc.arg(reporter_id)
ON CONFLICT (conversation_id, reporter_id) DO NOTHING
RETURNING id;

-- name: ListConversationReports :many
-- A page of reports, newest first, with the conversation's kind and the reporter and the
-- participant they reported, each as account id and persona. The persona turns the id
-- into the name the others saw; the id itself never leaves the service.
SELECT r.id,
       r.conversation_id,
       c.kind,
       r.reason,
       r.note,
       r.created_at,
       r.resolved_at,
       r.reporter_id,
       reporter.persona AS reporter_persona,
       other.user_id AS reported_id,
       other.persona AS reported_persona
FROM conversation_reports r
JOIN conversations c ON c.id = r.conversation_id
JOIN conversation_participants reporter ON reporter.conversation_id = r.conversation_id AND reporter.user_id = r.reporter_id
JOIN conversation_participants other ON other.conversation_id = r.conversation_id AND other.user_id <> r.reporter_id
WHERE sqlc.arg(include_resolved)::boolean OR r.resolved_at IS NULL
ORDER BY r.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListMessagesForConversations :many
-- Every message of the given conversations, oldest first within each, with the persona its
-- sender spoke as. One read for a whole page of reports.
SELECT m.conversation_id, m.sender_id, p.persona AS sender_persona, m.body, m.created_at
FROM messages m
JOIN conversation_participants p ON p.conversation_id = m.conversation_id AND p.user_id = m.sender_id
WHERE m.conversation_id = ANY(sqlc.arg(conversation_ids)::bigint[])
ORDER BY m.conversation_id, m.id;

-- name: ResolveConversationReport :one
-- Mark a report handled. An already resolved report keeps its first resolver and time.
UPDATE conversation_reports
SET resolved_at = coalesce(resolved_at, now()),
    resolved_by = coalesce(resolved_by, sqlc.arg(moderator_id)::bigint)
WHERE id = sqlc.arg(id)
RETURNING id;

-- name: ListUnreadDue :many
-- The participants due an unread notice: one row per (conversation, recipient). A
-- participant is due when another participant has written something past its read cursor,
-- the OLDEST such message is not newer than cutoff, and it was not already notified since
-- that message — notified_at is compared with the oldest unread message rather than
-- cleared on read, so reading and a later message re-arms it without the read path
-- knowing the worker exists. Open conversations notify every participant; a pending one
-- only those who did not start it, so a starter is never told about their own request.
-- Declined and closed are never due.
--
-- prev_notified_at and last_read are returned for the compare-and-set in ClaimUnread.
SELECT p.conversation_id,
       c.kind,
       p.user_id,
       (c.status = 'pending')::boolean AS pending,
       p.notified_at AS prev_notified_at,
       coalesce(p.last_read_message_id, 0)::bigint AS last_read,
       u.oldest_id,
       us.email,
       tl.chat_id
FROM conversation_participants p
JOIN conversations c ON c.id = p.conversation_id AND c.status IN ('open', 'pending')
JOIN users us ON us.id = p.user_id
LEFT JOIN telegram_links tl ON tl.user_id = p.user_id
CROSS JOIN LATERAL (
    SELECT msg.id AS oldest_id, msg.created_at AS oldest_at
    FROM messages msg
    WHERE msg.conversation_id = p.conversation_id
      AND msg.sender_id <> p.user_id
      AND msg.id > coalesce(p.last_read_message_id, 0)
    ORDER BY msg.id
    LIMIT 1
) u
WHERE (c.status = 'open' OR c.starter_id <> p.user_id)
  AND u.oldest_at <= sqlc.arg(cutoff)::timestamptz
  AND (p.notified_at IS NULL OR p.notified_at < u.oldest_at)
ORDER BY u.oldest_at
LIMIT sqlc.arg(max_rows);

-- name: ClaimUnread :one
-- Take the notice for one participant before sending it: only the run that moves the stamp
-- from the value it read (and sees the same read cursor) gets a row back. A concurrent
-- run, or a read that landed in between, gets none and sends nothing.
UPDATE conversation_participants p
SET notified_at = sqlc.arg(claimed_at)::timestamptz
FROM conversations c
WHERE p.conversation_id = sqlc.arg(conversation_id)
  AND p.user_id = sqlc.arg(user_id)
  AND c.id = p.conversation_id
  AND c.status IN ('open', 'pending')
  AND p.notified_at IS NOT DISTINCT FROM sqlc.narg(prev_notified_at)::timestamptz
  AND coalesce(p.last_read_message_id, 0) = sqlc.arg(last_read)::bigint
RETURNING p.conversation_id;

-- name: ReleaseUnread :exec
-- Give a claim back after every channel failed, so the next run retries. Guarded on the
-- stamp being the claim's own: a participant re-notified since keeps its newer one.
UPDATE conversation_participants
SET notified_at = sqlc.narg(prev_notified_at)::timestamptz
WHERE conversation_id = sqlc.arg(conversation_id)
  AND user_id = sqlc.arg(user_id)
  AND notified_at = sqlc.arg(claimed_at)::timestamptz;
