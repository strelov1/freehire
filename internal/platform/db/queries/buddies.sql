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
