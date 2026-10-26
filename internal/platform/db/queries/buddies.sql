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
-- Opt a member out: stamp left_at and close every pending or open buddy conversation they
-- are in, in one statement so a leave cannot half-apply. The conversations are the
-- messaging core's (kind 'buddy'); closing them here rather than through the core is what
-- keeps the two writes atomic. The row is kept, so rejoining restores the handle. The
-- member update is guarded by left_at IS NULL, so for a member who is not active it
-- returns no row; the conversation close is not guarded, which is harmless because a left
-- member has no pending or open buddy conversation to close.
WITH closed AS (
    UPDATE conversations c
    SET status = 'closed', closed_at = now()
    WHERE c.kind = 'buddy'
      AND c.status IN ('pending', 'open')
      AND EXISTS (
          SELECT 1 FROM conversation_participants p
          WHERE p.conversation_id = c.id AND p.user_id = sqlc.arg(user_id))
)
UPDATE buddy_members bm
SET left_at = now()
WHERE bm.user_id = sqlc.arg(user_id) AND bm.left_at IS NULL
RETURNING bm.*;

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
-- Excluded: the viewer, members who left, and any pair in user_blocks (account-wide) in either
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
          SELECT 1 FROM user_blocks b
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
      -- A privately pasted posting is its author's own: its employer does not count.
      AND NOT EXISTS (SELECT 1 FROM jobs pj WHERE pj.id = e.job_id AND pj.is_private)
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
-- either direction, events on a private (user-pasted) posting, and applications their author hid. The author's earlier events count
-- from before they joined: only left_at gates, never joined_at.
--
-- authors_only restricts to author_handles: the viewer's top similar members, which the
-- caller takes from ListSimilarBuddies so the scoring formula lives in one place, or the
-- one member whose page the viewer is reading. Either way the author still passes every
-- predicate above, so naming a handle never widens what the viewer may see. The day
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
  -- A privately pasted posting is visible to its creator only: its event never reaches the feed.
  AND (j.id IS NULL OR NOT j.is_private)
  AND e.user_id <> sqlc.arg(viewer_id)
  AND (NOT sqlc.arg(authors_only)::bool OR m.handle = ANY(sqlc.arg(author_handles)::text[]))
  AND (sqlc.narg(cursor_id)::bigint IS NULL
       OR (e.occurred_at, e.id) <= (SELECT c.occurred_at, c.id FROM application_events c WHERE c.id = sqlc.narg(cursor_id)::bigint))
  AND NOT EXISTS (
      SELECT 1 FROM user_blocks b
      WHERE (b.blocker_id = sqlc.arg(viewer_id) AND b.blocked_id = e.user_id)
         OR (b.blocker_id = e.user_id AND b.blocked_id = sqlc.arg(viewer_id)))
  AND NOT EXISTS (
      SELECT 1 FROM buddy_hidden_applications h
      WHERE h.user_id = e.user_id AND h.application_id = e.application_id)
ORDER BY e.occurred_at DESC, e.id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListBuddyMemberActivity :many
-- One member's feed events counted per UTC day since since_day, oldest first, for the year
-- grid on their page. It is ListBuddyFeed's visibility predicate RESTATED, narrowed to one
-- author and aggregated: sqlc cannot share a WHERE clause between two queries, and every
-- exclusion above must hold here too or the grid counts what the feed hides. The guard
-- against the two drifting is TestMemberActivityMatchesMemberFeed, which seeds every
-- exclusion and asserts the grid's total equals the author-filtered feed's length — change
-- one predicate and change the other. Only days with an event come back.
SELECT (e.occurred_at AT TIME ZONE 'UTC')::date AS day,
       count(*)::int AS events
FROM application_events e
JOIN buddy_members m ON m.user_id = e.user_id AND m.left_at IS NULL
LEFT JOIN jobs j ON j.id = e.job_id
WHERE e.kind IN ('applied', 'stage_set') AND e.retracted_at IS NULL
  AND (e.kind = 'applied' OR e.signal IN ('interview', 'offer'))
  AND (j.id IS NULL OR NOT j.is_private)
  AND e.user_id <> sqlc.arg(viewer_id)
  AND e.user_id = sqlc.arg(author_id)
  AND e.occurred_at >= sqlc.arg(since_day)::date::timestamp AT TIME ZONE 'UTC'
  AND NOT EXISTS (
      SELECT 1 FROM user_blocks b
      WHERE (b.blocker_id = sqlc.arg(viewer_id) AND b.blocked_id = e.user_id)
         OR (b.blocker_id = e.user_id AND b.blocked_id = sqlc.arg(viewer_id)))
  AND NOT EXISTS (
      SELECT 1 FROM buddy_hidden_applications h
      WHERE h.user_id = e.user_id AND h.application_id = e.application_id)
GROUP BY day
ORDER BY day;

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

-- name: ListBuddyCounterparts :many
-- How the other side of each of the viewer's buddy conversations is drawn: handle and card
-- inputs, and whether they may be shown at all. They may while both sides are active
-- members and no block stands between them in either direction; otherwise the
-- conversation still lists (its history is the viewer's) but the caller must not serve
-- the other member's handle or card.
SELECT m.user_id,
       m.handle,
       m.countries,
       u.resume_structured,
       (m.left_at IS NULL
        AND EXISTS (SELECT 1 FROM buddy_members v
                    WHERE v.user_id = sqlc.arg(viewer_id) AND v.left_at IS NULL)
        AND NOT EXISTS (
            SELECT 1 FROM user_blocks b
            WHERE (b.blocker_id = sqlc.arg(viewer_id) AND b.blocked_id = m.user_id)
               OR (b.blocker_id = m.user_id AND b.blocked_id = sqlc.arg(viewer_id))))::boolean AS visible
FROM buddy_members m
JOIN users u ON u.id = m.user_id
WHERE m.user_id = ANY(sqlc.arg(user_ids)::bigint[]);

-- name: ListBuddyHandles :many
-- The handles behind account ids, active or left, for a moderator reading a reported
-- buddy conversation by what its members saw.
SELECT user_id, handle FROM buddy_members WHERE user_id = ANY(sqlc.arg(user_ids)::bigint[]);
