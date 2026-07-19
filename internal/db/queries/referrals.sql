-- name: CreateReferralOffer :one
-- Record a member's offer to refer into a company. The UNIQUE (user_id, company_slug)
-- constraint rejects a second offer for the same company; the repository maps that unique
-- violation to a domain "already offered" error. Starts pending, awaiting moderation.
INSERT INTO referral_offers (user_id, company_slug, proof_object_key)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListReferralOffersByUser :many
-- The "my offers" list: one member's offers with moderation status, newest first.
SELECT * FROM referral_offers
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListPendingReferralOffers :many
-- The moderator queue: offers awaiting a decision, oldest first.
SELECT * FROM referral_offers
WHERE status = 'pending'
ORDER BY created_at;

-- name: DecideReferralOffer :one
-- Approve or reject a pending offer, recording the deciding moderator and time. The
-- status='pending' guard makes the decision idempotent-safe: a second decision on an
-- already-decided offer matches no row (the repository maps that to "not pending").
UPDATE referral_offers
SET status = sqlc.arg(status), decided_by = sqlc.arg(decided_by), decided_at = now()
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: CompanyHasApprovedReferrer :one
-- Whether a company is referral-eligible — has at least one approved offer. Served by
-- referral_offers_company_approved_idx.
SELECT EXISTS (
    SELECT 1 FROM referral_offers WHERE company_slug = $1 AND status = 'approved'
) AS exists;

-- name: CompaniesWithApprovedReferrer :many
-- The subset of the given company slugs that are referral-eligible — for annotating a
-- job/company list in one round-trip instead of a query per row.
SELECT DISTINCT company_slug FROM referral_offers
WHERE status = 'approved' AND company_slug = ANY(sqlc.arg(slugs)::text[]);

-- name: ReferrerApprovedForCompany :one
-- Whether a specific member is an approved referrer for a company — the authorization
-- check for acting on / viewing a request in that company's pool.
SELECT EXISTS (
    SELECT 1 FROM referral_offers
    WHERE user_id = sqlc.arg(user_id) AND company_slug = sqlc.arg(company_slug) AND status = 'approved'
) AS exists;

-- name: ListApprovedReferrerRecipients :many
-- The notify fan-out targets: every approved referrer of a company with their email and
-- linked Telegram chat (NULL when unlinked). Email is always present; chat_id drives the
-- optional Telegram ping.
SELECT o.user_id, u.email, t.chat_id
FROM referral_offers o
JOIN users u ON u.id = o.user_id
LEFT JOIN telegram_links t ON t.user_id = o.user_id
WHERE o.company_slug = $1 AND o.status = 'approved';

-- name: UserHasResume :one
-- Whether a user has a stored original résumé — the check before attaching an 'original'
-- CV to a request, so a seeker cannot request with a résumé they never uploaded.
SELECT EXISTS (
    SELECT 1 FROM users WHERE id = $1 AND resume_object_key IS NOT NULL AND resume_object_key <> ''
) AS exists;

-- name: GetReferralOffer :one
-- One offer by id — for the moderator's proof-CV view after role authorization.
SELECT * FROM referral_offers WHERE id = $1;

-- name: CVBelongsToUser :one
-- Whether a builder CV is owned by a user — the authorization check before attaching a
-- 'built' CV to a request, so a seeker cannot reference someone else's cv_id.
SELECT EXISTS (
    SELECT 1 FROM cvs WHERE id = sqlc.arg(cv_id) AND user_id = sqlc.arg(user_id)
) AS exists;

-- name: CreateReferralRequest :one
-- Record a seeker's referral request into a company. The partial unique index on
-- (seeker_user_id, company_slug) WHERE status='sent' rejects a second active request for
-- the same company; the repository maps that unique violation to "already requested".
INSERT INTO referral_requests (
    seeker_user_id, company_slug, job_id, cv_kind, cv_id,
    contact_telegram, contact_email, note
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListReferralRequestsBySeeker :many
-- The seeker's "my requests" list: their requests with current status, newest first.
SELECT * FROM referral_requests
WHERE seeker_user_id = $1
ORDER BY created_at DESC;

-- name: ListIncomingReferralRequests :many
-- The referrer inbox: open (sent) requests for every company the referrer has an approved
-- offer for. Joins the request pool to the caller's approved offers on company_slug.
SELECT r.* FROM referral_requests r
JOIN referral_offers o ON o.company_slug = r.company_slug
WHERE o.user_id = sqlc.arg(referrer_user_id) AND o.status = 'approved' AND r.status = 'sent'
ORDER BY r.created_at DESC;

-- name: GetReferralRequest :one
-- One referral request by id — for authorized CV access and marking, after the caller is
-- verified as an approved referrer of the request's company.
SELECT * FROM referral_requests WHERE id = $1;

-- name: ResolveReferralRequest :one
-- Mark a sent request contacted or declined, recording the acting referrer and time. The
-- status='sent' guard makes it race-safe: whichever referrer acts first wins; a second
-- attempt matches no row (mapped to "already resolved").
UPDATE referral_requests
SET status = sqlc.arg(status), acted_by = sqlc.arg(acted_by), acted_at = now()
WHERE id = sqlc.arg(id) AND status = 'sent'
RETURNING *;

-- name: CountReferralRequestsSince :one
-- How many requests a seeker has created since a cutoff — the per-day cap check.
SELECT count(*) FROM referral_requests
WHERE seeker_user_id = sqlc.arg(seeker_user_id) AND created_at >= sqlc.arg(since);
