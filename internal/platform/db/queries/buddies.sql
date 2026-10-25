-- name: JoinBuddyMember :one
-- Opt a member in to the interview-buddies network, or bring a returning one back. A first
-- join stores the handle the caller minted; a rejoin (the user_id conflict) clears left_at
-- and refreshes the ranking keys but leaves the stored handle alone, so the argument is
-- only used on the insert path. A handle collision is a different unique violation (the
-- handle column) that ON CONFLICT (user_id) does not absorb, so the caller can retry with
-- a new handle.
INSERT INTO buddy_members (user_id, handle, category, countries, regions)
VALUES (
    sqlc.arg(user_id), sqlc.arg(handle), sqlc.narg(category),
    coalesce(sqlc.arg(countries)::text[], '{}'), coalesce(sqlc.arg(regions)::text[], '{}')
)
ON CONFLICT (user_id) DO UPDATE
SET left_at   = NULL,
    category  = EXCLUDED.category,
    countries = EXCLUDED.countries,
    regions   = EXCLUDED.regions
RETURNING *;

-- name: LeaveBuddyMember :one
-- Opt a member out: stamp left_at and close every pending or open conversation they are
-- in, in one statement so a leave cannot half-apply. The row is kept, so rejoining
-- restores the handle. The member update is guarded by left_at IS NULL, so for a member
-- who is not active it returns no row; the conversation close is not guarded, which is
-- harmless because a left member has no pending or open conversation to close.
WITH closed AS (
    UPDATE buddy_conversations
    SET status = 'closed', closed_at = now()
    WHERE status IN ('pending', 'open')
      AND (user_low = sqlc.arg(user_id) OR user_high = sqlc.arg(user_id))
)
UPDATE buddy_members
SET left_at = now()
WHERE user_id = sqlc.arg(user_id) AND left_at IS NULL
RETURNING *;

-- name: GetBuddyMemberByUser :one
-- A member's own row, active or left. The caller decides what left_at means.
SELECT * FROM buddy_members WHERE user_id = $1;

-- name: GetBuddyMemberByHandle :one
-- The member behind a handle, active or left.
SELECT * FROM buddy_members WHERE handle = $1;

-- name: UpdateBuddyMemberRankingKeys :exec
-- Refresh the category and geography similarity is scored on. Skips a left member: their
-- keys are not read, and a write would only touch a row nobody is using.
UPDATE buddy_members
SET category  = sqlc.narg(category),
    countries = coalesce(sqlc.arg(countries)::text[], '{}'),
    regions   = coalesce(sqlc.arg(regions)::text[], '{}')
WHERE user_id = sqlc.arg(user_id) AND left_at IS NULL;

-- name: ListSimilarBuddies :many
-- The active members most like the viewer, best first, one page. The score is
--   w_category*(same category) + w_countries*(countries overlap) + w_regions*(regions
--   overlap) + w_company*(shared companies, at most company_cap counted)
-- with the weights passed in by the caller (they are named constants in Go, a first guess,
-- and this is the only place the formula lives). Ties break on joined_at DESC, then user_id
-- so a page boundary is stable. A member who shares nothing still ranks, with score 0.
--
-- A company is shared when BOTH sides applied to it (kind 'applied', not retracted) inside
-- the last 90 days, ignoring any application its own author has hidden from the buddy
-- feed: hiding an application takes it out of every buddy surface. The page's shared
-- companies come back as parallel slug / name arrays, at most company_cap, in slug order.
--
-- Excluded: the viewer, members who left, and any pair in buddy_blocks in either
-- direction. A viewer who is not an active member yields no rows (the service checks first).
-- The CV is read for the page's rows only, to project the card summary.
WITH viewer AS (
    SELECT vm.category, vm.countries, vm.regions
    FROM buddy_members vm
    WHERE vm.user_id = sqlc.arg(viewer_id) AND vm.left_at IS NULL
),
viewer_companies AS (
    SELECT DISTINCT e.company_slug
    FROM application_events e
    WHERE e.user_id = sqlc.arg(viewer_id)
      AND e.kind = 'applied' AND e.retracted_at IS NULL
      AND e.company_slug <> ''
      AND e.occurred_at > now() - interval '90 days'
      AND NOT EXISTS (
          SELECT 1 FROM buddy_hidden_applications h
          WHERE h.user_id = e.user_id AND h.application_id = e.application_id)
),
candidates AS (
    SELECT m.user_id, m.handle, m.category, m.countries, m.regions, m.joined_at
    FROM buddy_members m
    WHERE m.left_at IS NULL
      AND m.user_id <> sqlc.arg(viewer_id)
      AND NOT EXISTS (
          SELECT 1 FROM buddy_blocks b
          WHERE (b.blocker_id = sqlc.arg(viewer_id) AND b.blocked_id = m.user_id)
             OR (b.blocker_id = m.user_id AND b.blocked_id = sqlc.arg(viewer_id)))
),
shared AS (
    SELECT c.user_id, e.company_slug
    FROM candidates c
    JOIN application_events e ON e.user_id = c.user_id
    WHERE e.kind = 'applied' AND e.retracted_at IS NULL
      AND e.occurred_at > now() - interval '90 days'
      AND e.company_slug IN (SELECT company_slug FROM viewer_companies)
      AND NOT EXISTS (
          SELECT 1 FROM buddy_hidden_applications h
          WHERE h.user_id = e.user_id AND h.application_id = e.application_id)
    GROUP BY c.user_id, e.company_slug
),
shared_counts AS (
    SELECT user_id, count(*)::int AS n FROM shared GROUP BY user_id
),
scored AS (
    SELECT c.user_id, c.handle, c.countries, c.joined_at,
           (sqlc.arg(w_category)::int * coalesce(c.category = v.category, false)::int
          + sqlc.arg(w_countries)::int * (c.countries && v.countries)::int
          + sqlc.arg(w_regions)::int * (c.regions && v.regions)::int
          + sqlc.arg(w_company)::int * least(coalesce(sc.n, 0), sqlc.arg(company_cap)::int))::int AS score
    FROM candidates c
    CROSS JOIN viewer v
    LEFT JOIN shared_counts sc ON sc.user_id = c.user_id
),
page AS (
    SELECT * FROM scored
    ORDER BY score DESC, joined_at DESC, user_id
    LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset)
)
SELECT p.handle,
       p.countries,
       p.score,
       u.resume_structured,
       coalesce((SELECT array_agg(s.company_slug ORDER BY s.company_slug)
                 FROM (SELECT company_slug FROM shared WHERE shared.user_id = p.user_id
                       ORDER BY company_slug LIMIT sqlc.arg(company_cap)::int) s), '{}')::text[] AS shared_slugs,
       coalesce((SELECT array_agg(coalesce(co.name, s.company_slug) ORDER BY s.company_slug)
                 FROM (SELECT company_slug FROM shared WHERE shared.user_id = p.user_id
                       ORDER BY company_slug LIMIT sqlc.arg(company_cap)::int) s
                 LEFT JOIN companies co ON co.slug = s.company_slug), '{}')::text[] AS shared_names
FROM page p
JOIN users u ON u.id = p.user_id
ORDER BY p.score DESC, p.joined_at DESC, p.user_id;

-- name: ListBuddyFeed :many
-- One page of the buddy feed, newest first, keyset-paged on (occurred_at, id). Three kinds
-- of event qualify and nothing else: an application ('applied'), and a move into the
-- 'interview' or 'offer' stage. 'accepted' is excluded on purpose (it says the person
-- changed jobs) and so is every rejection, withdrawal and expiry. The kind / retracted_at
-- predicate is written exactly as the partial index application_events_feed_idx declares
-- it, which is what lets the planner use that index; the signal test only narrows it.
--
-- Excluded: the viewer's own events, authors who are not active members, blocked pairs in
-- either direction, and applications their author hid. The author's earlier events count
-- from before they joined: only left_at gates, never joined_at.
--
-- similar_only restricts to author_handles, the viewer's top similar members, which the
-- caller takes from ListSimilarBuddies so the scoring formula lives in one place. The day
-- is the UTC calendar date; the timestamp itself never leaves this query except as the
-- cursor key. cursor_id is NULL for the first page. Otherwise it names the last event of
-- the previous page and the page is read INCLUSIVE of it: the keyset position is resolved
-- here, from the event itself, so the client never holds a timestamp, and the caller
-- checks the first row is that event. Because the event passes through the same
-- visibility predicate as every other row, a cursor naming an event the viewer may not
-- see (rejected, hidden, blocked, retracted, unknown) comes back without it.
SELECT e.id,
       (e.occurred_at AT TIME ZONE 'UTC')::date AS day,
       e.kind,
       e.signal,
       e.company_slug,
       coalesce(co.name, e.company_slug)::text AS company_name,
       j.title AS job_title,
       m.handle,
       m.countries,
       u.resume_structured
FROM application_events e
JOIN buddy_members m ON m.user_id = e.user_id AND m.left_at IS NULL
JOIN users u ON u.id = e.user_id
LEFT JOIN jobs j ON j.id = e.job_id
LEFT JOIN companies co ON co.slug = e.company_slug
WHERE e.kind IN ('applied', 'stage_set') AND e.retracted_at IS NULL
  AND (e.kind = 'applied' OR e.signal IN ('interview', 'offer'))
  AND e.user_id <> sqlc.arg(viewer_id)
  AND (NOT sqlc.arg(similar_only)::bool OR m.handle = ANY(sqlc.arg(author_handles)::text[]))
  AND (sqlc.narg(cursor_id)::bigint IS NULL
       OR (e.occurred_at, e.id) <= (SELECT c.occurred_at, c.id FROM application_events c WHERE c.id = sqlc.narg(cursor_id)::bigint))
  AND NOT EXISTS (
      SELECT 1 FROM buddy_blocks b
      WHERE (b.blocker_id = sqlc.arg(viewer_id) AND b.blocked_id = e.user_id)
         OR (b.blocker_id = e.user_id AND b.blocked_id = sqlc.arg(viewer_id)))
  AND NOT EXISTS (
      SELECT 1 FROM buddy_hidden_applications h
      WHERE h.user_id = e.user_id AND h.application_id = e.application_id)
ORDER BY e.occurred_at DESC, e.id DESC
LIMIT sqlc.arg(page_limit);

-- name: HideBuddyApplication :one
-- Mark one of the caller's own applications as hidden from the buddy feed and from the
-- shared-company count. The SELECT is what proves ownership: for an application that is
-- not the caller's it yields nothing and no row comes back. Hiding twice is a no-op that
-- still returns the id (the DO UPDATE touches nothing real, it only makes RETURNING fire).
INSERT INTO buddy_hidden_applications (user_id, application_id)
SELECT a.user_id, a.id FROM applications a
WHERE a.id = sqlc.arg(application_id) AND a.user_id = sqlc.arg(user_id)
ON CONFLICT (user_id, application_id) DO UPDATE SET created_at = buddy_hidden_applications.created_at
RETURNING application_id;

-- name: ShowBuddyApplication :exec
-- Lift a hide. Keyed on the caller, so it can only ever remove the caller's own mark.
DELETE FROM buddy_hidden_applications
WHERE user_id = $1 AND application_id = $2;

-- name: ListHiddenBuddyApplications :many
-- The caller's hidden application ids, for the tracker's visibility switch.
SELECT application_id FROM buddy_hidden_applications
WHERE user_id = $1
ORDER BY application_id;

-- name: CreateBuddyInvitation :one
-- A pending conversation and its first message in ONE statement, so the pair never exists
-- without the message that opened it. The caller passes the pair already ordered
-- (user_low < user_high). A second conversation for the pair is the unique violation on
-- (user_low, user_high).
WITH c AS (
    INSERT INTO buddy_conversations (user_low, user_high, initiator_id)
    VALUES (sqlc.arg(user_low), sqlc.arg(user_high), sqlc.arg(initiator_id))
    RETURNING *
), m AS (
    INSERT INTO buddy_messages (conversation_id, sender_id, body)
    SELECT c.id, c.initiator_id, sqlc.arg(body) FROM c
)
SELECT * FROM c;

-- name: GetBuddyConversation :one
SELECT * FROM buddy_conversations WHERE id = $1;

-- name: GetBuddyConversationByPair :one
-- The pair's conversation in any status. The caller passes the pair ordered.
SELECT * FROM buddy_conversations WHERE user_low = $1 AND user_high = $2;

-- name: CountBuddyInvitationsSince :one
-- The initiator's invitations inside the rolling window, and when the oldest of them was
-- sent (NULL when there are none): that instant plus the window is when a slot frees up.
SELECT count(*)::bigint AS sent, min(created_at)::timestamptz AS oldest
FROM buddy_conversations
WHERE initiator_id = $1 AND created_at > $2;

-- name: BuddyPairBlocked :one
SELECT EXISTS (
    SELECT 1 FROM buddy_blocks
    WHERE (blocker_id = sqlc.arg(a) AND blocked_id = sqlc.arg(b))
       OR (blocker_id = sqlc.arg(b) AND blocked_id = sqlc.arg(a))
) AS blocked;

-- name: AnswerBuddyInvitation :one
-- Accept or decline a pending invitation. Only the party who did not send it matches; no
-- row means the caller must look the conversation up to say why.
UPDATE buddy_conversations
SET status = sqlc.arg(status)::text,
    accepted_at = CASE WHEN sqlc.arg(status)::text = 'open' THEN now() ELSE accepted_at END
WHERE id = sqlc.arg(id)
  AND status = 'pending'
  AND initiator_id <> sqlc.arg(user_id)
  AND (user_low = sqlc.arg(user_id) OR user_high = sqlc.arg(user_id))
RETURNING *;

-- name: InsertBuddyMessage :one
-- A message into an OPEN conversation the sender belongs to, and the sender's read cursor
-- moved onto it, in one statement. The status is guarded here, under FOR SHARE, and not by
-- an earlier read: a close that commits while this waits makes the row stop matching, so
-- no message lands after it. No row means the caller must look the conversation up to say
-- why.
WITH m AS (
    INSERT INTO buddy_messages (conversation_id, sender_id, body)
    SELECT c.id, sqlc.arg(sender_id)::bigint, sqlc.arg(body)::text
    FROM buddy_conversations c
    WHERE c.id = sqlc.arg(conversation_id)
      AND c.status = 'open'
      AND sqlc.arg(sender_id)::bigint IN (c.user_low, c.user_high)
    FOR SHARE OF c
    RETURNING id, conversation_id, sender_id, body, created_at
), r AS (
    UPDATE buddy_conversations c
    SET last_read_low  = CASE WHEN c.user_low  = m.sender_id THEN m.id ELSE c.last_read_low  END,
        last_read_high = CASE WHEN c.user_high = m.sender_id THEN m.id ELSE c.last_read_high END
    FROM m
    WHERE c.id = m.conversation_id
)
SELECT id, sender_id, body, created_at FROM m;

-- name: ListBuddyMessages :many
-- Messages after a given id, oldest first. Readability is the caller's check.
SELECT id, sender_id, body, created_at
FROM buddy_messages
WHERE conversation_id = sqlc.arg(conversation_id) AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(page_limit);

-- name: MarkBuddyConversationRead :one
-- Move the caller's side of the read cursor up to up_to, never back and never past the
-- conversation's newest message. No row means the caller is not a party.
UPDATE buddy_conversations c
SET last_read_low = CASE WHEN c.user_low = sqlc.arg(user_id)
        THEN GREATEST(coalesce(c.last_read_low, 0), LEAST(sqlc.arg(up_to)::bigint, newest.id))
        ELSE c.last_read_low END,
    last_read_high = CASE WHEN c.user_high = sqlc.arg(user_id)
        THEN GREATEST(coalesce(c.last_read_high, 0), LEAST(sqlc.arg(up_to)::bigint, newest.id))
        ELSE c.last_read_high END
FROM (
    SELECT coalesce(max(id), 0)::bigint AS id
    FROM buddy_messages WHERE conversation_id = sqlc.arg(conversation_id)
) newest
WHERE c.id = sqlc.arg(conversation_id)
  AND sqlc.arg(user_id) IN (c.user_low, c.user_high)
RETURNING c.id;

-- name: ListBuddyConversations :many
-- Every conversation of the viewer with the other member's card inputs, their unread count
-- (the other side's messages past the viewer's read cursor) and the newest message time,
-- newest first. A declined invitation is hidden from the member who declined it. The
-- unread count and last time come from one pass over the conversation's messages, so
-- the list costs one query however many conversations there are.
SELECT c.id,
       c.user_low,
       c.user_high,
       c.initiator_id,
       c.status,
       c.created_at,
       c.accepted_at,
       m.handle,
       m.countries,
       u.resume_structured,
       stats.unread::bigint AS unread,
       coalesce(stats.newest, c.created_at)::timestamptz AS last_message_at
FROM buddy_conversations c
JOIN buddy_members m ON m.user_id = CASE WHEN c.user_low = sqlc.arg(viewer_id) THEN c.user_high ELSE c.user_low END
JOIN users u ON u.id = m.user_id
CROSS JOIN LATERAL (
    SELECT count(*) FILTER (
               WHERE msg.sender_id <> sqlc.arg(viewer_id)
                 AND msg.id > coalesce(CASE WHEN c.user_low = sqlc.arg(viewer_id) THEN c.last_read_low ELSE c.last_read_high END, 0)
           ) AS unread,
           max(msg.created_at) AS newest
    FROM buddy_messages msg
    WHERE msg.conversation_id = c.id
) stats
WHERE sqlc.arg(viewer_id) IN (c.user_low, c.user_high)
  AND NOT (c.status = 'declined' AND c.initiator_id <> sqlc.arg(viewer_id))
ORDER BY last_message_at DESC, c.id DESC;

-- name: BlockBuddyConversation :one
-- Block the other party of a conversation and close it, in one statement so neither half
-- can land alone. Only a party matches; no row means the caller must say the conversation
-- is not theirs. Closing is allowed from any status, and a conversation that was already
-- closed keeps its original closed_at. Blocking twice writes nothing the second time.
WITH c AS (
    UPDATE buddy_conversations
    SET status = 'closed', closed_at = coalesce(closed_at, now())
    WHERE id = sqlc.arg(conversation_id)
      AND sqlc.arg(blocker_id)::bigint IN (user_low, user_high)
    RETURNING id, CASE WHEN user_low = sqlc.arg(blocker_id) THEN user_high ELSE user_low END AS other_id
), b AS (
    INSERT INTO buddy_blocks (blocker_id, blocked_id)
    SELECT sqlc.arg(blocker_id)::bigint, c.other_id FROM c
    ON CONFLICT DO NOTHING
)
SELECT id FROM c;

-- name: InsertBuddyReport :one
-- A report of a conversation by one of its parties. No row means either the reporter is
-- not a party or they already reported it; the caller looks the conversation up to say
-- which.
INSERT INTO buddy_reports (conversation_id, reporter_id, reason, note)
SELECT c.id, sqlc.arg(reporter_id)::bigint, sqlc.arg(reason)::text, sqlc.narg(note)::text
FROM buddy_conversations c
WHERE c.id = sqlc.arg(conversation_id)
  AND sqlc.arg(reporter_id)::bigint IN (c.user_low, c.user_high)
ON CONFLICT (conversation_id, reporter_id) DO NOTHING
RETURNING id;

-- name: ListBuddyReports :many
-- A page of reports, newest first, with the handles of the reporter and of the member
-- they reported. Handles only: a moderator reads the conversation by what the members saw.
SELECT r.id,
       r.conversation_id,
       r.reason,
       r.note,
       r.created_at,
       r.resolved_at,
       reporter.handle AS reporter_handle,
       other.handle    AS reported_handle
FROM buddy_reports r
JOIN buddy_conversations c ON c.id = r.conversation_id
JOIN buddy_members reporter ON reporter.user_id = r.reporter_id
JOIN buddy_members other ON other.user_id = CASE WHEN c.user_low = r.reporter_id THEN c.user_high ELSE c.user_low END
WHERE sqlc.arg(include_resolved)::boolean OR r.resolved_at IS NULL
ORDER BY r.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListBuddyMessagesForConversations :many
-- Every message of the given conversations, oldest first within each, with the sender's
-- handle. One read for a whole page of reports.
SELECT msg.conversation_id, m.handle AS sender_handle, msg.body, msg.created_at
FROM buddy_messages msg
JOIN buddy_members m ON m.user_id = msg.sender_id
WHERE msg.conversation_id = ANY(sqlc.arg(conversation_ids)::bigint[])
ORDER BY msg.conversation_id, msg.id;

-- name: ResolveBuddyReport :one
-- Mark a report handled. An already resolved report keeps its first resolver and time.
UPDATE buddy_reports
SET resolved_at = coalesce(resolved_at, now()),
    resolved_by = coalesce(resolved_by, sqlc.arg(moderator_id)::bigint)
WHERE id = sqlc.arg(id)
RETURNING id;

-- name: ListBuddyUnreadDue :many
-- The sides of conversations that are due an unread notice (design D9). One row per
-- (conversation, recipient side). A side is due when the other participant has written
-- something past this side's read cursor, the OLDEST such message is not newer than
-- sqlc.arg(cutoff), and the side was not already notified since that message — notified_*_at
-- is compared with the oldest unread message rather than cleared on read, so reading and a
-- later message re-arms it without the read path knowing the worker exists. Open
-- conversations notify both sides; a pending one only its recipient, so an initiator is
-- never told about their own invitation. Declined and closed are never due, and neither is
-- a member who has left.
--
-- prev_notified_at and last_read are returned for the compare-and-set in ClaimBuddyUnread.
SELECT s.conversation_id,
       s.user_id,
       s.is_low,
       (s.status = 'pending')::boolean AS invitation,
       s.notified_at AS prev_notified_at,
       s.last_read,
       u.oldest_id,
       us.email,
       tl.chat_id
FROM (
    SELECT c.id AS conversation_id, c.status, c.initiator_id,
           c.user_low AS user_id, c.user_high AS other_id, true AS is_low,
           c.last_read_low AS last_read, c.notified_low_at AS notified_at
    FROM buddy_conversations c
    WHERE c.status IN ('open', 'pending')
    UNION ALL
    SELECT c.id, c.status, c.initiator_id,
           c.user_high, c.user_low, false,
           c.last_read_high, c.notified_high_at
    FROM buddy_conversations c
    WHERE c.status IN ('open', 'pending')
) s
JOIN buddy_members bm ON bm.user_id = s.user_id AND bm.left_at IS NULL
JOIN users us ON us.id = s.user_id
LEFT JOIN telegram_links tl ON tl.user_id = s.user_id
CROSS JOIN LATERAL (
    SELECT msg.id AS oldest_id, msg.created_at AS oldest_at
    FROM buddy_messages msg
    WHERE msg.conversation_id = s.conversation_id
      AND msg.sender_id = s.other_id
      AND msg.id > coalesce(s.last_read, 0)
    ORDER BY msg.id
    LIMIT 1
) u
WHERE (s.status = 'open' OR s.initiator_id <> s.user_id)
  AND u.oldest_at <= sqlc.arg(cutoff)::timestamptz
  AND (s.notified_at IS NULL OR s.notified_at < u.oldest_at)
ORDER BY u.oldest_at
LIMIT sqlc.arg(max_rows);

-- name: ClaimBuddyUnread :one
-- Take the notice for one side before sending it: only the run that moves the stamp from
-- the value it read (and sees the same read cursor) gets a row back. A concurrent run, or
-- a read that landed in between, gets none and sends nothing.
UPDATE buddy_conversations c
SET notified_low_at  = CASE WHEN sqlc.arg(is_low)::boolean THEN sqlc.arg(claimed_at)::timestamptz ELSE c.notified_low_at END,
    notified_high_at = CASE WHEN sqlc.arg(is_low)::boolean THEN c.notified_high_at ELSE sqlc.arg(claimed_at)::timestamptz END
WHERE c.id = sqlc.arg(conversation_id)
  AND c.status IN ('open', 'pending')
  AND CASE WHEN sqlc.arg(is_low)::boolean
        THEN c.notified_low_at IS NOT DISTINCT FROM sqlc.narg(prev_notified_at)::timestamptz
         AND coalesce(c.last_read_low, 0) = sqlc.arg(last_read)::bigint
        ELSE c.notified_high_at IS NOT DISTINCT FROM sqlc.narg(prev_notified_at)::timestamptz
         AND coalesce(c.last_read_high, 0) = sqlc.arg(last_read)::bigint
      END
RETURNING c.id;

-- name: ReleaseBuddyUnread :exec
-- Give a claim back after every channel failed, so the next run retries. Guarded on the
-- stamp being the claim's own: a side that has since been re-notified keeps its newer one.
UPDATE buddy_conversations c
SET notified_low_at  = CASE WHEN sqlc.arg(is_low)::boolean THEN sqlc.narg(prev_notified_at)::timestamptz ELSE c.notified_low_at END,
    notified_high_at = CASE WHEN sqlc.arg(is_low)::boolean THEN c.notified_high_at ELSE sqlc.narg(prev_notified_at)::timestamptz END
WHERE c.id = sqlc.arg(conversation_id)
  AND CASE WHEN sqlc.arg(is_low)::boolean
        THEN c.notified_low_at = sqlc.arg(claimed_at)::timestamptz
        ELSE c.notified_high_at = sqlc.arg(claimed_at)::timestamptz
      END;
