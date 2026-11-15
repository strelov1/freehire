-- A company account's team: its members and the invitations that bring them in
-- (migration 0271). The claimant stays the account (company_accounts); members act for it.

-- name: GetCompanyAccountForUser :one
-- The account a user acts for, as its claimant or as a member, in whatever status the account
-- is in. acting_email is the user's own work address — the claimant's work_email, or the one
-- the member's invitation was sent to — because it is where that user's vacancies'
-- applications go and what candidates reply to. member tells the two apart. Two primary-key
-- lookups rather than one OR across a join, which neither key could serve; a user is in at
-- most one of the two (the writes keep it so), so at most one row comes back.
SELECT ca.user_id, ca.company_slug, ca.company_name, ca.status, ca.verified_at,
       ca.email_confirmed_at, ca.created_at, ca.work_email AS acting_email, false AS member
FROM company_accounts ca
WHERE ca.user_id = sqlc.arg(user_id)
UNION ALL
SELECT ca.user_id, ca.company_slug, ca.company_name, ca.status, ca.verified_at,
       ca.email_confirmed_at, ca.created_at, m.work_email AS acting_email, true AS member
FROM company_account_members m
JOIN company_accounts ca ON ca.company_slug = m.company_slug
WHERE m.user_id = sqlc.arg(user_id);

-- name: EmployerTeamIncludes :one
-- Whether other is actor or acts for the same company as actor (employer_team, migration
-- 0271). Every user is their own teammate, so a caller never special-cases itself.
SELECT (sqlc.arg(actor_id)::bigint = sqlc.arg(other_id)::bigint OR EXISTS (
    SELECT 1 FROM employer_team(sqlc.arg(actor_id)::bigint) AS t(user_id)
    WHERE t.user_id = sqlc.arg(other_id)::bigint
))::boolean AS includes;

-- name: LockEmployerActor :exec
-- Serialises the two writes that make a user act for a company — a claim and an invitation's
-- acceptance — for one user. Each refuses a user the other table already holds, but two
-- concurrent statements would each check a snapshot without the other's row; taken first in
-- the same transaction, this lock makes the second write's check see the first's commit.
SELECT pg_advisory_xact_lock(hashtextextended('employer_actor:' || sqlc.arg(user_id)::bigint::text, 0));

-- name: LockActingMembership :one
-- Locks the user's membership row (FOR KEY SHARE) and names its company. A vacancy write
-- takes it so that removing the member — or deleting their company account, which cascades
-- here — waits for the write to commit, and then hands over or detaches the new vacancy
-- too. No row: the user is no member (the claimant, or nobody).
SELECT company_slug FROM company_account_members WHERE user_id = sqlc.arg(user_id) FOR KEY SHARE;

-- name: LockCompanyAccountBySlug :one
-- Locks a company account row (FOR KEY SHARE), for the same reason: deleting the account
-- waits for an in-flight vacancy write of its team. Taken BEFORE LockActingMembership, the
-- order the account's deletion takes the two. Returns the claimant. No row: the account is
-- gone.
SELECT user_id FROM company_accounts WHERE company_slug = sqlc.arg(company_slug) FOR KEY SHARE;

-- name: LockAuthor :one
-- Locks the author's users row (FOR KEY SHARE) for an employer vacancy write, before the
-- company and membership rows: deleting the user takes that row first and then cascades to
-- the company account and memberships, so the write takes the three in the same order and
-- the two serialise instead of deadlocking (the write's created_by foreign-key check would
-- otherwise wait on the user row while holding the account row). No row: the user is gone.
SELECT id FROM users WHERE id = sqlc.arg(user_id) FOR KEY SHARE;

-- name: ListCompanyTeam :many
-- Everyone who acts for a company: the claimant first (holder), then members by when they
-- joined. The claimant joined when the account was verified.
SELECT t.user_id, t.work_email, t.joined_at, t.holder
FROM (
    SELECT ca.user_id, ca.work_email, COALESCE(ca.verified_at, ca.created_at) AS joined_at, true AS holder
    FROM company_accounts ca
    WHERE ca.company_slug = sqlc.arg(company_slug)
    UNION ALL
    SELECT m.user_id, m.work_email, m.joined_at, false AS holder
    FROM company_account_members m
    WHERE m.company_slug = sqlc.arg(company_slug)
) t
ORDER BY t.holder DESC, t.joined_at, t.user_id;

-- name: ListCompanyInvitations :many
-- A company's invitations that can still be accepted, newest first.
SELECT id, email, created_at, expires_at
FROM company_account_invitations
WHERE company_slug = sqlc.arg(company_slug) AND expires_at > now()
ORDER BY created_at DESC;

-- name: UpsertCompanyInvitation :one
-- Invites an address to a company's team, or replaces the token and expiry of the live or
-- expired invitation already sent to it — a resend leaves exactly one working link. It is
-- refused (no row) once the company holds max_live invitations that can still be accepted,
-- unless it renews one that is itself still live; reviving an expired one needs a free slot.
-- The cap bounds how much mail one account can make freehire send. Callers hold the
-- company's lock (LockEmployerCompany), so two invitations cannot both pass the count.
INSERT INTO company_account_invitations (company_slug, email, token_hash, invited_by, expires_at)
SELECT sqlc.arg(company_slug), sqlc.arg(email), sqlc.arg(token_hash), sqlc.arg(invited_by), sqlc.arg(expires_at)
WHERE EXISTS (
        SELECT 1 FROM company_account_invitations
        WHERE company_slug = sqlc.arg(company_slug) AND lower(email) = lower(sqlc.arg(email))
          AND expires_at > now()
    )
   OR (
        SELECT count(*) FROM company_account_invitations
        WHERE company_slug = sqlc.arg(company_slug) AND expires_at > now()
    ) < sqlc.arg(max_live)::bigint
ON CONFLICT (company_slug, lower(email)) DO UPDATE
SET email = EXCLUDED.email, token_hash = EXCLUDED.token_hash, invited_by = EXCLUDED.invited_by,
    created_at = now(), expires_at = EXCLUDED.expires_at
RETURNING id, email, created_at, expires_at;

-- name: DeleteCompanyInvitation :execrows
-- Withdraws one of a company's invitations; its link stops working at once.
DELETE FROM company_account_invitations
WHERE id = sqlc.arg(id) AND company_slug = sqlc.arg(company_slug);

-- name: GetCompanyInvitationByToken :one
-- The live invitation a link names, with the company and the status of the account it
-- invites to. No row: no such token (never issued, already accepted, withdrawn or replaced by
-- a resend), or it has expired.
SELECT i.email, i.company_slug, ca.company_name, ca.status
FROM company_account_invitations i
JOIN company_accounts ca ON ca.company_slug = i.company_slug
WHERE i.token_hash = sqlc.arg(token_hash) AND i.expires_at > now();

-- name: AcceptCompanyInvitation :one
-- Consumes a live invitation to an active account and makes user_id a member, in one
-- statement, so a link cannot be accepted twice. Refused (no row) when the token is unknown
-- or expired, the account is not active, or the user already acts for a company — as its
-- claimant here, or as a member through company_account_members' primary key, which a
-- concurrent acceptance hits as a unique violation.
WITH inv AS (
    DELETE FROM company_account_invitations i
    USING company_accounts ca
    WHERE i.token_hash = sqlc.arg(token_hash)
      AND i.expires_at > now()
      AND ca.company_slug = i.company_slug
      AND ca.status = 'active'
      AND NOT EXISTS (SELECT 1 FROM company_accounts own WHERE own.user_id = sqlc.arg(user_id))
    RETURNING i.company_slug, i.email, i.invited_by
)
INSERT INTO company_account_members (user_id, company_slug, work_email, invited_by)
SELECT sqlc.arg(user_id), inv.company_slug, inv.email, inv.invited_by FROM inv
RETURNING company_slug;

-- name: LockCompanyMemberForRemoval :one
-- Locks a member row FOR UPDATE ahead of RemoveCompanyMember, in its own statement: it waits
-- for an in-flight vacancy write holding the row FOR KEY SHARE to commit, so the removal's
-- statement that follows takes a snapshot that sees the new vacancy and hands it over too
-- (one statement waiting inside its DELETE would keep its earlier snapshot for the hand-over
-- and miss it). No row: not a member of this company.
SELECT user_id FROM company_account_members
WHERE user_id = sqlc.arg(member_id) AND company_slug = sqlc.arg(company_slug)
FOR UPDATE;

-- name: RemoveCompanyMember :one
-- Removes a member from a company's team, and in the same statement hands the vacancies they
-- published to the account's claimant — a vacancy belongs to the team, and one left created
-- by somebody outside it would stop taking applications and drop out of every teammate's
-- dashboard — and withdraws the invitations they sent, which would otherwise let them invite
-- an address of their own and walk straight back in. The claimant is never a member row, so
-- cannot be removed here. Returns how many members were removed (0 or 1).
WITH removed AS (
    DELETE FROM company_account_members m
    WHERE m.user_id = sqlc.arg(member_id) AND m.company_slug = sqlc.arg(company_slug)
    RETURNING m.user_id AS removed_id
), handed AS (
    UPDATE jobs
    SET created_by = (SELECT ca.user_id FROM company_accounts ca WHERE ca.company_slug = sqlc.arg(company_slug))
    WHERE created_by IN (SELECT removed.removed_id FROM removed)
      AND source = 'employer'
), withdrawn AS (
    DELETE FROM company_account_invitations i
    WHERE i.invited_by IN (SELECT removed.removed_id FROM removed)
      AND i.company_slug = sqlc.arg(company_slug)
)
SELECT count(*)::bigint AS removed FROM removed;

-- name: LockEmployerCompany :exec
-- Serialises every change to one company's team — inviting, accepting, removing — so the
-- invitation cap holds, and a member being removed cannot have an invitation of theirs
-- inserted after the removal withdrew the others. Transaction-scoped.
SELECT pg_advisory_xact_lock(hashtextextended('employer_company:' || sqlc.arg(company_slug)::text, 0));

-- name: IsCompanyActor :one
-- Whether the user acts for the company — as its claimant or a member — right now. Read
-- under LockEmployerCompany, so a removal that committed first is seen.
SELECT EXISTS (
    SELECT 1 FROM company_accounts ca WHERE ca.company_slug = sqlc.arg(company_slug) AND ca.user_id = sqlc.arg(user_id)
    UNION ALL
    SELECT 1 FROM company_account_members m WHERE m.company_slug = sqlc.arg(company_slug) AND m.user_id = sqlc.arg(user_id)
)::boolean AS acts;

-- name: DeleteCompanyInvitationByToken :execrows
-- Withdraws the invitation a failed letter carried — only while it still holds that token:
-- a resend that replaced the token in the meantime mailed a link that must keep working.
DELETE FROM company_account_invitations
WHERE company_slug = sqlc.arg(company_slug) AND token_hash = sqlc.arg(token_hash);
