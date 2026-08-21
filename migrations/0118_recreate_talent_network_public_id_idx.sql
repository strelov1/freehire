-- migrate: no-transaction
--
-- Recreates users_talent_network_public_id_key, which 0117 dropped because prod's copy
-- was INVALID. Same definition as 0086 — this is a repair, not a redesign, so the shape
-- must not drift: a bare unique index (no named UNIQUE table constraint), which is what
-- 0086 chose and what credit_ledger_reward_ref_uniq (0041) established as the precedent
-- for a concurrently-built uniqueness guard.
--
-- It restores both things the invalid index was failing to do: the uniqueness of the
-- public profile UUID, and an index scan for `WHERE u.talent_network_public_id = $1`
-- instead of the sequential scan on users that EXPLAIN showed on prod.
--
-- CONCURRENTLY in its own no-transaction file, same reasoning as 0086/0101. On an
-- existing prod volume, run it DETACHED from the SSH session (systemd-run or nohup):
-- attached is how the original build died and left the invalid index this pair exists
-- to clean up. Re-running after an interrupted attempt needs the 0117 drop first,
-- because a half-built index is present-but-invalid and IF NOT EXISTS would skip it.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS users_talent_network_public_id_key
    ON public.users (talent_network_public_id);
