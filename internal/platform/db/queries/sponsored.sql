-- name: CreateSponsoredCampaign :one
INSERT INTO sponsored_campaigns (job_id, owner_user_id, cpc_cents, daily_cap_cents)
VALUES ($1, $2, $3, sqlc.narg(daily_cap_cents))
RETURNING *;

-- name: GetSponsoredCampaignForUpdate :one
-- Locks the campaign row: every charge and every state change serializes here.
SELECT * FROM sponsored_campaigns WHERE id = $1 FOR UPDATE;

-- name: SetSponsoredCampaignStatus :exec
UPDATE sponsored_campaigns SET status = $2, updated_at = now() WHERE id = $1;

-- name: ChargeSponsoredCampaign :one
-- Adds a click's cost to spent_cents and flips the campaign to exhausted when the budget is
-- gone. The WHERE re-checks the budget, so even a caller that skipped the lock cannot overspend.
UPDATE sponsored_campaigns
SET spent_cents = spent_cents + sqlc.arg(cost_cents)::bigint,
    status = CASE WHEN spent_cents + sqlc.arg(cost_cents)::bigint >= budget_cents
                  THEN 'exhausted' ELSE status END,
    updated_at = now()
WHERE id = sqlc.arg(id) AND spent_cents + sqlc.arg(cost_cents)::bigint <= budget_cents
RETURNING *;

-- name: EndSponsoredCampaignsForJob :many
-- Ends every live campaign of a job (the job closed); unused budget stays recorded.
UPDATE sponsored_campaigns
SET status = 'ended', updated_at = now()
WHERE job_id = $1 AND status NOT IN ('rejected', 'ended')
RETURNING id;

-- name: HasRecentValidSponsoredClick :one
SELECT EXISTS (
    SELECT 1 FROM sponsored_clicks
    WHERE campaign_id = $1 AND visitor_hash = $2 AND valid AND ts > sqlc.arg(since)::timestamptz
);

-- name: InsertSponsoredClick :exec
INSERT INTO sponsored_clicks (campaign_id, ts, placement, visitor_hash, valid, reason, cost_cents)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetSponsoredDailySpent :one
SELECT COALESCE((SELECT spent_cents FROM sponsored_daily WHERE campaign_id = $1 AND day = $2), 0)::bigint;

-- name: UpsertSponsoredDaily :exec
INSERT INTO sponsored_daily (campaign_id, day, impressions, valid_clicks, invalid_clicks, spent_cents)
VALUES ($1, $2, sqlc.arg(impressions), sqlc.arg(valid_clicks), sqlc.arg(invalid_clicks), sqlc.arg(spent_cents))
ON CONFLICT (campaign_id, day) DO UPDATE
SET impressions    = sponsored_daily.impressions    + EXCLUDED.impressions,
    valid_clicks   = sponsored_daily.valid_clicks   + EXCLUDED.valid_clicks,
    invalid_clicks = sponsored_daily.invalid_clicks + EXCLUDED.invalid_clicks,
    spent_cents    = sponsored_daily.spent_cents    + EXCLUDED.spent_cents;

-- name: ListSponsoredCampaignsByOwner :many
SELECT * FROM sponsored_campaigns WHERE owner_user_id = $1 ORDER BY id DESC;

-- name: ListSponsoredReviewQueue :many
SELECT * FROM sponsored_campaigns WHERE status = 'pending_review' ORDER BY updated_at, id;

-- name: InsertSponsoredTopup :one
-- ON CONFLICT DO NOTHING on the session id is the whole of the webhook idempotency: no row
-- back means this payment was already recorded.
INSERT INTO sponsored_topups (campaign_id, stripe_session_id, payment_intent_id, amount_cents, credited)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (stripe_session_id) DO NOTHING
RETURNING id;

-- name: CreditSponsoredBudget :exec
UPDATE sponsored_campaigns
SET budget_cents = budget_cents + sqlc.arg(amount_cents)::bigint, status = $2, updated_at = now()
WHERE id = $1;

-- name: ReviewSponsoredCampaign :exec
-- Moderator decision: stamps who and when; reject_reason is NULL on approval.
UPDATE sponsored_campaigns
SET status = $2, reviewed_by = $3, reviewed_at = now(), reject_reason = sqlc.narg(reject_reason), updated_at = now()
WHERE id = $1;

-- name: ListSponsoredTopupsToRefund :many
-- Money that must go back: every top-up of a rejected campaign, and any top-up that arrived
-- for a campaign that could no longer serve (credited = false). A NULL campaign_id scans all.
SELECT t.id, t.campaign_id, t.payment_intent_id
FROM sponsored_topups t
JOIN sponsored_campaigns c ON c.id = t.campaign_id
WHERE t.refunded_at IS NULL
  AND t.payment_intent_id IS NOT NULL
  AND (c.status = 'rejected' OR NOT t.credited)
  AND (sqlc.narg(campaign_id)::bigint IS NULL OR t.campaign_id = sqlc.narg(campaign_id)::bigint)
ORDER BY t.id
LIMIT sqlc.arg(max_rows);

-- name: MarkSponsoredTopupRefunded :exec
UPDATE sponsored_topups SET refunded_at = now() WHERE id = $1 AND refunded_at IS NULL;
