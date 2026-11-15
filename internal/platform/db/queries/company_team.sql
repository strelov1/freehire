-- A company account's team: who acts for it
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
-- waits for an in-flight vacancy write of its team. No row: the account is gone.
SELECT company_slug FROM company_accounts WHERE company_slug = sqlc.arg(company_slug) FOR KEY SHARE;

-- name: LockCompanyAccountByUser :one
-- LockCompanyAccountBySlug for the claimant, addressed by user. No row: no claim.
SELECT company_slug FROM company_accounts WHERE user_id = sqlc.arg(user_id) FOR KEY SHARE;
