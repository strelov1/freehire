# Discount conventions

## Scope

What discount an account is owed on its next subscription invoice, and nothing else. Three
surfaces, one mechanism: a promo code redeemed at checkout, the first-month discount an
invited account is offered, and the credit a referrer earns once the person they invited
actually pays.

This package never talks to the payment provider. `internal/identity/billing` does, imports
nothing from here, and is imported by nothing here — the two meet in
`internal/api/handler/billing.go` for a checkout and in `cmd/billing-sync` for a reward,
and both of those may import anything.

**It is NOT `internal/engage/referral`.** That is the employee-referral marketplace — offers
to refer somebody into a company, moderation, anonymity between seeker and referrer. It
shares a word with this feature and nothing else, and the account section calls this one
"Invite a friend" for the same reason.

## Always true

- **Every value that decides money is a row or an environment variable.** The repository is
  public: a seat limit or a launch code readable from the source is drained the day somebody
  greps it. Two tests in `nocodes_test.go` enforce it — no literal the `promo_codes` CHECK
  would accept appears in the discount sources, and nothing anywhere `INSERT`s into that
  table. An operator's `INSERT` is the only interface, which is also what makes
  `UPDATE promo_codes SET active = false` a rollback that needs no deploy.

- **The launch offer is a row, not a feature.** "90% off for early adopters" is one
  `promo_codes` row with a seat limit. There is no second mechanism, no date constant, and
  nothing in the source that knows the offer exists.

- **A reward is earned by money, never by a signup.** The condition is ANY invoice with
  `amount_paid > 0` — any and not the first, so an invitee whose first month was free under
  a total discount and who then paid for a second still earns it. They paid us. A subscription can be active having collected nothing — a trial, a
  total discount — and rewarding that turns a discount into a way to mint credit. This is
  also the whole anti-abuse design: attribution is a cookie the visitor controls and is
  therefore free to forge, while a payment is not.

- **Every refusal about a CODE is one error** (`ErrNotUsable`): unknown, inactive, expired,
  out of seats. The preview route is authenticated but an account is free to create, so a
  refusal that distinguished "no such code" from "out of seats" would be an oracle for
  guessing codes. `ErrAlreadyRedeemed` is separate because it is about the CALLER, discloses
  nothing about any code, and is the answer that actually helps them.

- **The seat is claimed by the statement that tests it** (`RedeemPromoCode`, one statement).
  A read-then-write would let two accounts take the last seat of a launch offer, and the
  moment that matters is exactly the moment an offer is popular. The `NOT EXISTS` in the same
  `WHERE` is why a caller who is already ineligible does not even decrement `uses`; where two
  codes are redeemed concurrently by one account, the unique violation aborts the losing
  statement **whole**, seat increment included, because one statement is one subtransaction.

- **An account redeems one code in its lifetime.** `promo_redemptions` is keyed on `user_id`,
  not on `(code, user_id)`. Stripe admits one coupon per checkout session, so two offers held
  at once could not both be honoured anyway — and the version keyed on the pair would let an
  account collect offers it discovers at the till that it cannot spend.

- **An invitee is worth one reward for life**, enforced by `UNIQUE (referee_id)` rather than
  by a check. A check that runs before a write is a race; a constraint is not.

- **Attribution belongs to account CREATION, and the rule is in the SQL.**
  `AttributeInvite` inserts only for a user created within the hour. Without it an account
  two years old could open a friend's link, sign in, and collect a first-month discount plus
  a reward for the friend — a promo code with extra steps, and one nobody rationed. In SQL
  rather than in a handler so a caller who gets it wrong still cannot break it.

- **The code travels in a server-set cookie** (`AttributionCookie`, named here and read by
  the web app's request hook). Not browser storage: the majority sign-up path is OAuth, which
  returns on a GET redirect with no body a value could ride in. Server-set because Safari
  caps script-written cookies at seven days and the window is thirty.

- **Granting and delivering are a worker pass, not a webhook hook.** `cmd/billing-sync`,
  third pass. Applying a webhook event is answered inside a window the provider enforces and
  must not grow work that can fail; it is also provider-generic, and a reward is Stripe-only
  because a store subscription produces no invoice we can read. Both halves are guarded on
  the row's own state, so stopping the pass mid-way is free.

- **The stamp follows the credit, never precedes it.** Stamping first means a failure between
  the two records money that never moved, and nothing looks at that row again. The other way
  round costs one repeat, which the provider's idempotency key absorbs.

- **The amount is fixed when the reward is granted.** A later price change must not revalue
  credit somebody has already earned, in either direction.

- **`Stats` names nobody, and has nowhere to put a name.** Telling a referrer which of their
  contacts signed up discloses that a particular person is looking for work. The absence is
  a property of the type and of the query, not a filter at the edge.

## What is deliberately absent

**An admin UI for codes.** An `INSERT` is the interface. A UI would be a code path that
creates offers, which is the one thing the guards above forbid.

**An amount-off discount.** A referral reward is delivered as credit on the customer's
balance, never as a coupon: a coupon would have to be marked consumed when a session is
CREATED, and a checkout session is abandoned far more often than it is completed. Because of
that, only a percentage can reach a session, and the question of which of two discounts wins
does not arise.

**A ceiling in the database.** `INVITE_REWARD_MAX_PER_USER` is an environment variable,
like every other operational bound in this repository. A table holding one integer would be
a second place to look for a number.

**Anything for store subscriptions.** A subscription bought inside the App Store is
discounted inside the App Store, where we have no API. The RevenueCat path is untouched.
