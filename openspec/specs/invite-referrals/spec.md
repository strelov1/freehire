# invite-referrals Specification

## Purpose
A per-account invite link, the attribution of a signup to it, the invitee's first-month
discount, and the reward the referrer earns once that invitee has paid us at least what
the reward is worth.

## Requirements
### Requirement: Every account has one invite code, minted on demand

An account SHALL have at most one invite code. The code SHALL be generated from a
cryptographically secure source and be long enough that guessing one is not a way to find an
account. It is minted the first time the account asks for its invite link and never rotates.

#### Scenario: The account asks for its link twice

- **WHEN** an account requests its invite link, then requests it again
- **THEN** both responses carry the same code, and `invite_codes` holds one row for it

#### Scenario: Two accounts never share a code

- **WHEN** codes are minted for many accounts
- **THEN** every code is unique, enforced by a unique constraint rather than by retrying on
  a read

### Requirement: The invite link attributes a signup for thirty days

`/r/<code>` SHALL record the referrer in a cookie that expires in 30 days and redirect the
visitor to the site. An account created while that cookie is present SHALL be attributed to
the referrer named by it.

#### Scenario: A visitor follows the link and signs up later

- **WHEN** a visitor opens `/r/<code>`, browses, and registers within 30 days
- **THEN** an `invite_rewards` row is created linking the referrer to the new account with
  status `pending`

#### Scenario: The cookie has expired

- **WHEN** a visitor registers more than 30 days after opening the link, with no cookie left
- **THEN** no `invite_rewards` row is created and registration succeeds unchanged

#### Scenario: The code in the cookie is unknown

- **WHEN** a visitor registers carrying a cookie whose code matches no `invite_codes` row
- **THEN** no `invite_rewards` row is created and registration succeeds unchanged

#### Scenario: An existing account follows the link

- **WHEN** a visitor who already has an account opens `/r/<code>` and signs in
- **THEN** no `invite_rewards` row is created — attribution happens at account creation only

### Requirement: The code survives every path from the link to the account

The referrer's code SHALL be carried in a cookie written by the server, captured from a `ref`
query parameter on any request as well as from `/r/<code>`, and read by every registration
path including the OAuth callback.

#### Scenario: The link points at a deep page

- **WHEN** a visitor opens any page of the site with `?ref=<code>` appended
- **THEN** the attribution cookie is set, exactly as it would be for `/r/<code>`

#### Scenario: The visitor registers through an identity provider

- **WHEN** an attributed visitor signs up through OAuth, returning on a redirect that carries
  no request body
- **THEN** the callback reads the cookie and the attribution is recorded

#### Scenario: A second link tries to take over

- **WHEN** a visitor who already carries an attribution cookie opens a different invite link
- **THEN** the cookie keeps the first code

#### Scenario: Attribution fails

- **WHEN** writing the `invite_rewards` row fails during registration
- **THEN** the registration still succeeds and the failure is logged — a referral must never
  cost somebody their account

#### Scenario: Attribution succeeds

- **WHEN** the `invite_rewards` row is written during registration
- **THEN** the same response expires the attribution cookie

### Requirement: Nobody refers themselves, and an invitee is worth one reward for life

An `invite_rewards` row SHALL be refused when the referrer and the invitee are the same
account. The invitee SHALL be unique across the table, enforced by a unique constraint, so
that an account can never be the subject of a second reward.

#### Scenario: Self-referral

- **WHEN** an account's own invite cookie is present at its own registration
- **THEN** no row is written

#### Scenario: An account is re-attributed

- **WHEN** a second `invite_rewards` row naming an existing invitee is attempted
- **THEN** the write is refused by the constraint and the first row is untouched

### Requirement: The invitee is discounted on their first month

An account with a `pending` `invite_rewards` row SHALL be offered 50% off the first invoice
of the subscription it buys. The discount does not recur.

#### Scenario: The invitee subscribes

- **WHEN** an invited account starts checkout
- **THEN** the checkout session carries a 50%-off, once-only discount

#### Scenario: The invited subscription renews

- **WHEN** that subscription renews the following month
- **THEN** the renewal is charged at list price

### Requirement: The referrer's reward is earned by money, not by a signup

A reward SHALL move from `pending` to `granted` only once the invitee has an invoice that
actually collected — `amount_paid` greater than zero. A subscription that became active
without collecting anything, whether through a trial or a total discount, SHALL NOT earn a
reward.

#### Scenario: The invitee pays

- **WHEN** ANY of the invitee's invoices collects a non-zero amount
- **THEN** the reward becomes `granted` with an amount equal to 50% of the list price

#### Scenario: The first invoice was free but a renewal was not

- **WHEN** the invitee's first invoice collected nothing — a total discount — and a later
  renewal collects
- **THEN** the reward is granted, because the rule is that the invitee paid us, not that
  they paid us immediately

#### Scenario: The invitee's subscription is active but collected nothing

- **WHEN** the invitee's subscription is active and every invoice collected zero
- **THEN** the reward stays `pending`

#### Scenario: The signal arrives more than once

- **WHEN** the provider redelivers the event that carried the payment signal
- **THEN** the reward is granted exactly once and the referrer is credited exactly once

#### Scenario: A store purchase earns nothing

- **WHEN** the invitee subscribes through the App Store or Google Play
- **THEN** no reward is granted, because no invoice we can read collected anything

### Requirement: The reward is delivered as a credit, and a customer is created if needed

A granted reward SHALL be delivered as a credit on the referrer's provider customer, which
the provider consumes on that customer's next invoice. A referrer with no provider customer
SHALL have one created and bound before the credit is placed. A reward SHALL never be
delivered as a discount on a checkout session.

#### Scenario: The referrer is a subscriber

- **WHEN** a reward is granted for a referrer who has a provider customer
- **THEN** a credit for the reward amount is placed on that customer and the reward is
  stamped delivered

#### Scenario: The referrer has never bought anything

- **WHEN** a reward is granted for a referrer with no provider customer
- **THEN** a customer is created, bound to the account, credited, and the reward is stamped
  delivered

#### Scenario: The customer binding already exists

- **WHEN** delivery runs for a referrer whose account is already bound to a customer
- **THEN** no second customer is created and the existing binding is unchanged

#### Scenario: Delivery is retried

- **WHEN** the delivery pass runs again over a reward it has already delivered
- **THEN** no second credit is placed

### Requirement: A checkout session carries at most one discount

A checkout session SHALL carry at most one coupon, and the only discount that can reach a
session is a percentage — from a redeemed promo code or from a pending invite. Referral
credit never appears here.

#### Scenario: A redeemed promo code and an invite discount collide

- **WHEN** an invited account redeems a promo code and starts checkout
- **THEN** exactly one percentage discount is attached, and the response states which

#### Scenario: An account holding credit buys

- **WHEN** an account with accrued referral credit starts checkout
- **THEN** the session carries no discount from that credit, and the credit is consumed by
  the invoice the provider issues

### Requirement: Rewards per referrer are bounded

The number of rewards one referrer may earn SHALL be bounded by
`INVITE_REWARD_MAX_PER_USER`, defaulting to 12 when the variable is absent or unreadable.
Attributions beyond the bound are still recorded; they simply never grant.

#### Scenario: The referrer is at the ceiling

- **WHEN** an invitee of a referrer already holding the maximum granted rewards pays
- **THEN** the row stays `pending`, no credit is issued, and the invitee's own discount is
  unaffected

#### Scenario: The variable is missing

- **WHEN** the process starts with no `INVITE_REWARD_MAX_PER_USER` set
- **THEN** the ceiling is 12 and the subsystem starts normally

### Requirement: The invite page reports what the account has actually earned

`GET /me/invite` SHALL return the account's link, how many invitees it has, how many have
earned a reward, and the total credit earned. It SHALL NOT identify the invitees.

#### Scenario: A referrer with mixed invitees

- **WHEN** an account with three invitees, one of whom has paid, reads its invite page
- **THEN** the response reports three invitees, one reward, and the credit for one reward,
  and carries no invitee email, name or account id
