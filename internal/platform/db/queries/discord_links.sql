-- name: LinkDiscordAccount :one
-- Bind a Discord account to a freehire account, or move an existing binding to a different
-- Discord account.
--
-- ON CONFLICT on the user, not on the Discord account, and the difference is the whole
-- point. Re-linking your OWN account to a different Discord identity is an ordinary thing to
-- do — you changed Discord accounts — and it replaces the binding. Naming a Discord account
-- that SOMEBODY ELSE holds is the case the unique index on discord_user_id refuses (23505),
-- because letting it through would put the paid role on a second person for one
-- subscription. The caller reports that refusal as a conflict rather than retrying it.
--
-- The grant and the sync stamp are cleared on a move: they describe the account being left
-- behind, which still holds the role until the caller revokes it there. Keeping them would
-- make reconciliation believe the NEW account already has what it has not been given.
INSERT INTO discord_links (user_id, discord_user_id)
VALUES (sqlc.arg(user_id), sqlc.arg(discord_user_id)::text)
ON CONFLICT (user_id) DO UPDATE
SET discord_user_id = EXCLUDED.discord_user_id,
    linked_at       = now(),
    role_granted_at = NULL,
    synced_at       = NULL
RETURNING user_id, discord_user_id, linked_at, role_granted_at, synced_at;

-- name: GetDiscordLink :one
-- One account's binding, for the integrations surface and for the unlink path. No rows means
-- not linked, which every caller reports as a state rather than as an error.
SELECT user_id, discord_user_id, linked_at, role_granted_at, synced_at
FROM discord_links
WHERE user_id = $1;

-- name: UnlinkDiscordAccount :execrows
-- Remove a binding, reporting how many rows went.
--
-- The count is what lets the unlink route be idempotent without first reading: 0 means there
-- was nothing to remove, and that is a success, not a 404. A user who double-clicks
-- "Disconnect" must not see a failure for having got what they asked for.
DELETE FROM discord_links WHERE user_id = $1;

-- name: ListDiscordLinksToSync :many
-- The next page of bindings to reconcile, each with the plan that decides its role.
--
-- The plan comes back WITH the link, in one read, because resolving it separately would be
-- one query per account and could straddle a billing sync — an account that renewed
-- mid-page would be read as paying by one query and free by another.
--
-- ORDER BY synced_at NULLS FIRST makes this a rotating queue. The bound exists to keep a run
-- inside its timer, and with any fixed ordering the accounts past the bound would simply
-- never be examined; ordering by least-recently-examined means a bounded run that cannot
-- reach everybody still reaches everybody over successive runs, and a run that stops early
-- loses nothing because there is no cursor to lose. user_id breaks ties so the order is
-- total and two runs cannot interleave over the same row.
SELECT dl.user_id, dl.discord_user_id, dl.role_granted_at, u.pro_until, u.ultra_until
FROM discord_links dl
JOIN users u ON u.id = dl.user_id
ORDER BY dl.synced_at NULLS FIRST, dl.user_id
LIMIT $1;

-- name: SetDiscordRoleGranted :exec
-- Record the outcome of examining one binding: whether the role is now held, and that it was
-- examined just now.
--
-- ONE statement for both, because they are one fact. Written separately, a process that died
-- between them would either grant the role and leave the row at the front of the queue
-- forever, or stamp the row as done while forgetting what it did — and the second is the
-- dangerous one, because the next run would then see "role not granted" for an account that
-- has it and re-grant on every pass.
--
-- role_granted_at is passed NULL to say the role is not held: after a revocation, and after
-- Discord answers that the member is unknown to the guild. The stamp moves either way, so a
-- member who left does not pin the queue.
UPDATE discord_links
SET role_granted_at = sqlc.narg(role_granted_at),
    synced_at       = now()
WHERE user_id = sqlc.arg(user_id);

-- name: DeleteDiscordLinkForUser :exec
-- Erase one user's binding. Account deletion calls this alongside its other erasures; the
-- foreign key cascades, but deletion states what it erases explicitly rather than relying on
-- a constraint to mean it.
DELETE FROM discord_links WHERE user_id = $1;
