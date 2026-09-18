-- name: InsertPendingCompanyAccount :one
-- Reserves a company slug for a claim in progress. The UNIQUE(company_slug) constraint is
-- the whole race guard: two concurrent claims on the same slug both attempt this insert, one
-- succeeds, and the loser's caller maps the unique violation to a clean conflict — see
-- pgerr.IsUniqueViolation. user_id is the table's primary key, so a user who already holds
-- an account (in any status) also fails here, on the primary-key conflict.
INSERT INTO company_accounts (user_id, company_slug, company_name, work_email)
VALUES (@user_id, @company_slug, @company_name, @work_email)
RETURNING *;

-- name: GetCompanyAccountByUserID :one
-- The one account a user may hold, in whatever status it is in. Every employer-facing
-- capability check reads this and then inspects status itself, rather than a second query
-- per status, since a pending/revoked account needs to be distinguishable in the refusal.
SELECT *
FROM company_accounts
WHERE user_id = @user_id;

-- name: ListPendingCompanyAccounts :many
-- The moderator review queue: every claim that could not auto-activate (unknown or
-- mismatched work-email domain), oldest first so the queue drains in claim order.
SELECT *
FROM company_accounts
WHERE status = 'pending'
ORDER BY created_at;

-- name: ActivateCompanyAccount :one
-- Shared by both activation paths — the domain-match auto-activate and a moderator's
-- approval — since both are the same state transition. Scoped to status='pending' so
-- activating an already-active or revoked account is a no-op read miss (ErrNoRows) rather
-- than a silent re-stamp of verified_at.
UPDATE company_accounts
SET status = 'active', verified_at = now(), updated_at = now()
WHERE user_id = @user_id AND status = 'pending'
RETURNING *;

-- name: RevokeCompanyAccount :one
-- An admin kill switch. Deliberately does not delete the row or free company_slug — see the
-- migration comment: there is no self-service handoff on this MVP, so freeing the slug on
-- revoke would let anyone re-claim a company an admin just decided to shut out.
UPDATE company_accounts
SET status = 'revoked', updated_at = now()
WHERE user_id = @user_id AND status IN ('pending', 'active')
RETURNING *;

-- name: DeleteCompanyAccount :execrows
-- A moderator's rejection of a pending claim. Scoped to status='pending' so this can never
-- be used to erase an active or revoked account's record — rejection is a claim-time verdict,
-- not a way to un-revoke.
DELETE FROM company_accounts
WHERE user_id = @user_id AND status = 'pending';
