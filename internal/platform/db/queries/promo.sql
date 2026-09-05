-- name: PreviewPromoCode :one
-- Is this code usable right now? Read-only, and deliberately says nothing about WHY it is
-- not: the route behind it is rate limited but still reachable by anyone with an account,
-- and a refusal that distinguished "no such code" from "out of seats" would turn it into an
-- oracle for guessing codes. No rows means no.
--
-- The caller's own redemption history is checked separately, because that is a fact about
-- the caller rather than about the code, and telling them "you have already used a code" is
-- both useful and leaks nothing.
SELECT percent_off
FROM promo_codes
WHERE code = sqlc.arg(code)::text
  AND active
  AND (expires_at IS NULL OR expires_at > now())
  AND (max_uses IS NULL OR uses < max_uses);

-- name: RedeemPromoCode :one
-- Claim a seat and record the redemption, in ONE statement, returning the percentage.
--
-- One statement because it has to be atomic and a transaction here would be the wrong tool.
-- The seat is claimed by the same UPDATE that tests it, so two accounts racing for the last
-- seat of a launch offer cannot both win — a read-then-write would let them, and the moment
-- that matters is exactly the moment the offer is popular.
--
-- The NOT EXISTS is the other half: an account redeems one code in its lifetime, so the
-- claim must not even increment `uses` for somebody who is already ineligible. And where
-- two different codes are redeemed concurrently by ONE account, both may pass that check
-- against their own snapshot — the unique violation on promo_redemptions.user_id then aborts
-- the losing statement WHOLE, seat increment included, because a single statement is a
-- single subtransaction. That is the property this shape is buying.
--
-- No rows back means refused, without saying which bound refused it.
WITH claimed AS (
    UPDATE promo_codes
       SET uses = uses + 1
     WHERE code = sqlc.arg(code)::text
       AND active
       AND (expires_at IS NULL OR expires_at > now())
       AND (max_uses IS NULL OR uses < max_uses)
       AND NOT EXISTS (
           SELECT 1 FROM promo_redemptions pr WHERE pr.user_id = sqlc.arg(user_id)
       )
    RETURNING code, percent_off
), recorded AS (
    INSERT INTO promo_redemptions (user_id, code)
    SELECT sqlc.arg(user_id)::bigint, claimed.code FROM claimed
    RETURNING promo_redemptions.user_id
)
SELECT claimed.percent_off FROM claimed, recorded;

-- name: HasRedeemedPromoCode :one
-- Whether this account has already spent its one redemption. Asked after a refusal, to turn
-- the deliberately vague "no" above into the one explanation that is about the caller.
SELECT EXISTS (SELECT 1 FROM promo_redemptions WHERE user_id = $1);

-- name: RedeemedPromoPercent :one
-- What the code this account redeemed is worth, for the checkout that is about to attach a
-- coupon. Separate from the EXISTS above because a refusal needs only the fact while a
-- checkout needs the number, and reading the join to answer a yes/no question would be
-- work for its own sake on a path that runs far more often.
SELECT c.percent_off
FROM promo_redemptions r
JOIN promo_codes c ON c.code = r.code
WHERE r.user_id = $1;

-- name: EnsureInviteCode :one
-- The account's invite code, minted on first ask and never rotated.
--
-- ON CONFLICT DO UPDATE rather than DO NOTHING, because DO NOTHING returns no row and the
-- caller wants the code whether or not this call is the one that created it. The self-assign
-- is the idiomatic way to make the existing row visible to RETURNING.
--
-- The generated code can also collide on its own unique index. That conflict is NOT handled
-- here: it means crypto/rand produced a value already held, which at this width is not a
-- case to write code around — the caller retries and the second draw succeeds.
INSERT INTO invite_codes (user_id, code)
VALUES (sqlc.arg(user_id), sqlc.arg(code)::text)
ON CONFLICT (user_id) DO UPDATE SET user_id = invite_codes.user_id
RETURNING code;

-- name: ReferrerByInviteCode :one
-- Who owns this invite code. No rows means a code nobody minted, which the attribution path
-- treats as no attribution rather than as an error: the value came out of a cookie, and a
-- cookie is whatever the visitor put in it.
SELECT user_id FROM invite_codes WHERE code = sqlc.arg(code)::text;

-- name: AttributeInvite :execrows
-- Record that this account arrived through that account's link.
--
-- ON CONFLICT DO NOTHING on the invitee, so a second attribution of the same account writes
-- nothing rather than failing: an account is worth one reward for its whole life, and the
-- table's unique constraint is what says so. Self-referral is refused by the table's own
-- check constraint too; the service refuses it earlier so the common case is not an error.
--
-- The SELECT rather than a VALUES is the freshness rule, and it is here rather than in Go
-- deliberately. Attribution belongs to account CREATION: without this, an account that has
-- existed for two years could open a friend's link, sign in, and collect a first-month
-- discount plus a reward for its friend — which is a promo code with extra steps, and one
-- nobody rationed. An hour is far longer than a sign-up takes and far shorter than a
-- second visit, and being in SQL means a caller who gets the rule wrong still cannot break
-- it.
INSERT INTO invite_rewards (referrer_id, referee_id)
SELECT sqlc.arg(referrer_id), u.id
FROM users u
WHERE u.id = sqlc.arg(referee_id)
  AND u.created_at > now() - interval '1 hour'
ON CONFLICT (referee_id) DO NOTHING;

-- name: PendingInviteRewards :many
-- The worker's grant pass: attributed signups that could plausibly have paid.
--
-- Narrowed to invitees who hold a provider customer, because that binding is written when a
-- purchase is first recorded — without one there is nothing to ask the provider about, and
-- asking anyway would be one API call per person who signed up and never bought, which is
-- almost all of them.
SELECT r.id, r.referrer_id, r.referee_id, u.stripe_customer_id
FROM invite_rewards r
JOIN users u ON u.id = r.referee_id
WHERE r.status = 'pending'
  AND u.stripe_customer_id IS NOT NULL
ORDER BY r.id
LIMIT sqlc.arg(max_rows);

-- name: CountGrantedInviteRewards :one
-- How many rewards this referrer has already earned, for the per-referrer ceiling. Counts
-- granted rows only: an attribution that never paid costs nothing and must not use up a slot.
SELECT count(*) FROM invite_rewards
WHERE referrer_id = $1 AND status = 'granted';

-- name: GrantInviteReward :execrows
-- Move one reward to granted at the amount it is worth today, if the referrer is below the
-- ceiling.
--
-- Guarded on the current status, which is what makes the pass idempotent: a re-run over a
-- row somebody else already granted affects no rows, and the caller reads that as "already
-- done" rather than doing it twice. The amount is fixed here and never recomputed, so a
-- later price change cannot revalue credit that has been earned.
--
-- The CEILING is counted inside this statement, and that is NOT what makes it a bound —
-- worth saying plainly, because it looks like it should be. This statement locks the reward
-- row it updates and nothing else, so two passes granting DIFFERENT pending rewards of one
-- referrer never block each other, and under READ COMMITTED each subquery reads the snapshot
-- its own statement began with. Both would see eleven against a ceiling of twelve, and both
-- would grant.
--
-- What actually bounds it is the advisory lock the referral pass takes (cmd/billing-sync,
-- key 0x66687277, registered in internal/platform/migrate). The count here is the cheap
-- second guard: with the pass serialized it is always right, and if the lock is ever lost
-- the damage is one extra reward rather than an unbounded run.
UPDATE invite_rewards
SET status = 'granted', granted_at = now(), amount_cents = sqlc.arg(amount_cents)
WHERE invite_rewards.id = sqlc.arg(id)
  AND invite_rewards.status = 'pending'
  AND (
      SELECT count(*) FROM invite_rewards earned
       WHERE earned.referrer_id = invite_rewards.referrer_id
         AND earned.status = 'granted'
  ) < sqlc.arg(ceiling);

-- name: UndeliveredInviteRewards :many
-- The worker's delivery pass: earned, but not yet placed on the referrer's balance.
--
-- Unlike the grant pass this does NOT require the referrer to hold a customer. A referrer
-- who has never bought anything has one created for them, because the alternative — holding
-- the reward until their own checkout — meant marking credit consumed by a session that is
-- abandoned far more often than it is completed.
SELECT r.id, r.referrer_id, r.amount_cents
FROM invite_rewards r
WHERE r.status = 'granted' AND r.delivered_at IS NULL
ORDER BY r.id
LIMIT sqlc.arg(max_rows);

-- name: MarkInviteRewardDelivered :execrows
-- Stamp a reward as placed on the referrer's balance. Guarded on the stamp being absent, so
-- a repeat affects no rows. The credit itself carries an idempotency key on the provider's
-- side, so the two guards fail in the same direction: never twice.
UPDATE invite_rewards
SET delivered_at = now()
WHERE id = $1 AND delivered_at IS NULL;

-- name: InviteStats :one
-- What the account's own invite page says: how many people came through the link, how many
-- of them earned a reward, and what that adds up to.
--
-- Aggregates only. There is deliberately no query that lists an account's invitees: naming
-- who accepted an invite discloses that a particular person signed up for a job board, which
-- is not the referrer's to know.
SELECT count(*) AS invitees,
       count(*) FILTER (WHERE status = 'granted') AS rewarded,
       coalesce(sum(amount_cents), 0)::bigint AS credit_cents
FROM invite_rewards
WHERE referrer_id = $1;

-- name: PendingInviteDiscount :one
-- Does this account still owe itself the invitee's first-month discount?
--
-- `pending` is the whole condition: once the reward is granted the invitee has paid, and a
-- first-month discount they have already been past is not owed again.
SELECT EXISTS (
    SELECT 1 FROM invite_rewards WHERE referee_id = $1 AND status = 'pending'
);
