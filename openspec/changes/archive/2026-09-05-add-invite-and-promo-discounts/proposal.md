## Why

Pro sells at a single price with no lever to move it. There is no way to run a launch
offer, thank the people who paid before the product deserved it, or let a subscriber bring
someone in — every acquisition costs the same as the last, and the cheapest channel a paid
product has (an existing subscriber telling someone) is not wired up at all.

The repository is public, which decides the shape more than the marketing does: a discount
mechanism whose codes, limits or reward rules can be read out of the source is a discount
mechanism that gets drained. Every figure that decides money lives in the database or the
host environment, and the source carries only the rules for reading them.

## What Changes

- **A promo code redeemed at checkout.** A row in `promo_codes` names a percentage off the
  first month, an optional seat limit, and an expiry. The launch offer for early adopters is
  one such row — 90% off, capped seats — not a second mechanism and not a
  constant in the source.
- **A personal invite link per account.** `/r/<code>` remembers the referrer for 30 days.
  Whoever signs up through it gets 50% off their first month.
- **A reward for the referrer, paid on money, not on signups.** When ANY of the invitee's
  invoices actually collects (`amount_paid > 0`), the referrer earns 50% of the list price.
  Any, not the first: an invitee whose first month was free under a total discount and who
  then paid for a second has still paid us. A
  reward is always a credit on the referrer's provider customer, which the next invoice
  consumes — and a referrer who has never bought has a customer created for them, because a
  reward held until their own checkout would have to be marked consumed by a session that is
  abandoned far more often than it is completed. Reward credit never becomes a coupon.
- **One discount per checkout session, and one code per account for life.** The provider
  admits a single coupon per session, and stacking a promo code onto a referral discount
  onto accrued credit is how a $5 subscription becomes free by accident.
- **A new `internal/identity/promo` package** that decides what discount an account is owed.
  It does not talk to the provider and `internal/identity/billing` does not import it: the
  HTTP layer holds them together, so billing's scope ("the Pro subscription, nothing else")
  stays true.
- **Abuse bounds that hold without trusting the client**: no self-referral, one reward per
  invitee for life (enforced by a unique constraint, not by a check), a per-referrer reward
  ceiling from the host environment, and an authenticated, rate-limited code preview — a
  four-character code left open to anonymous guessing is enumerated in an afternoon.
- **Out of scope: store subscriptions.** A subscription bought inside the App Store is
  changed and discounted inside the App Store, where we have no API. Discounts are a Stripe
  and web concern; the RevenueCat path is untouched.

## Capabilities

### New Capabilities

- `promo-codes`: A named discount stored in the database that an account may redeem once,
  bounded by seats and expiry, applied to the first invoice of a new subscription.
- `invite-referrals`: A per-account invite link, the attribution of a signup to it, the
  invitee's first-month discount, and the referrer's reward earned when the invitee pays.

### Modified Capabilities

None. `pro-subscription` is still an unarchived change, so its requirements are not yet in
`openspec/specs/`; this change adds an optional discount to the checkout it describes
without altering how a subscription entitles anyone.

## Impact

- **Schema**: new tables `promo_codes`, `promo_redemptions`, `invite_codes`,
  `invite_rewards`. One new migration; nothing applied is edited. `make sqlc` regenerates
  `internal/platform/db`.
- **Go**: new package `internal/identity/promo`, registered in the layering table at
  `internal/platform/arch/layering/blocks.go` (a package in neither table fails depguard).
  `internal/identity/billing` gains two provider abilities it does not currently have —
  attaching a discount to a checkout session and crediting a customer's balance — plus a way
  to answer whether an account has ever paid. Its callers, not it, decide when to use them.
- **HTTP**: `GET /api/v1/me/invite`, `POST /api/v1/me/promo/preview` (rate limited), and an
  optional code on the existing checkout route. A public `/r/<code>` redirect that sets the
  attribution cookie.
- **Web**: a promo-code field on `web/src/routes/pricing/+page.svelte` and an invite page
  showing the account's link, how many invitees signed up and paid, and the credit accrued.
- **Environment**: `INVITE_REWARD_MAX_PER_USER` (default 12) on the host. Absent, the
  default applies; the subsystem never fails to start over it.
- **Operations**: promo codes are created by `INSERT`, not by deploy. No code ships in the
  repository, and a test enforces that.
