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
-- restores the handle. left_at IS NULL makes it a no-op (no row) for a member who is
-- not currently active.
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
