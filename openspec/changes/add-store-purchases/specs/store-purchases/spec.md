## ADDED Requirements

### Requirement: The plan column is derived from its sources, never written

The system SHALL store each origin's reach in its own column — `users.pro_until_stripe`,
`users.pro_until_revenuecat`, `users.pro_until_granted` — and SHALL derive
`users.pro_until` as the latest of them. `users.pro_until` SHALL NOT be assignable.

This supersedes `pro-subscription`'s requirement "The plan column has exactly two writers",
which is stated here rather than as a delta because `pro-subscription` is not yet in
`openspec/specs/`. The guarantee it was protecting survives and is strengthened: the column
now has no writers at all, and each source has exactly one.

#### Scenario: An account with no source is on the free plan

- **WHEN** all three source columns are NULL
- **THEN** `pro_until` is NULL and plan resolution returns the free tier

#### Scenario: One source confers the plan

- **WHEN** exactly one source column holds a future instant
- **THEN** `pro_until` equals that instant

#### Scenario: The longest reach wins

- **WHEN** two source columns hold different future instants
- **THEN** `pro_until` equals the later of them

#### Scenario: The column cannot be assigned directly

- **WHEN** a statement attempts `UPDATE users SET pro_until = …`
- **THEN** the statement fails, and no row is modified

### Requirement: No provider can revoke another source's grant

The system SHALL write only the column belonging to the provider whose state was read. A
provider that reports no subscription SHALL clear its own column and no other.

The failure this prevents is specific and silent: an account that bought Pro in the App
Store has no Stripe subscription, so a Stripe sync derives the zero time — and before this
change would have written it over the entitlement Apple was paid for.

#### Scenario: A Stripe sync leaves a store purchase standing

- **GIVEN** an account whose `pro_until_revenuecat` is a future instant and who has never
  transacted with Stripe
- **WHEN** the Stripe sync runs for that account
- **THEN** `pro_until_stripe` is NULL, `pro_until_revenuecat` is unchanged, and `pro_until`
  still holds the store purchase's instant

#### Scenario: A RevenueCat sync leaves a web subscription standing

- **GIVEN** an account whose `pro_until_stripe` is a future instant and who holds no store
  subscription
- **WHEN** the RevenueCat sync runs for that account
- **THEN** `pro_until_revenuecat` is NULL and `pro_until` still holds the Stripe instant

#### Scenario: A manual grant survives every provider sync

- **GIVEN** an account whose `pro_until_granted` is a future instant
- **WHEN** either provider's sync runs and reports no subscription
- **THEN** `pro_until_granted` is unchanged and the account remains on Pro

### Requirement: Existing plan values are separated by the origin that set them

The migration SHALL place each existing non-NULL `pro_until` into exactly one source
column, chosen by the only evidence available: an account holding a
`users.stripe_customer_id` was set by the Stripe sync, and an account without one was set by
hand.

#### Scenario: A Stripe subscriber's value stays revocable

- **WHEN** the migration runs against an account with a `stripe_customer_id` and a non-NULL
  `pro_until`
- **THEN** the value lands in `pro_until_stripe`, where a later cancellation can clear it

#### Scenario: A hand-set value stops being revocable by a provider

- **WHEN** the migration runs against an account with no `stripe_customer_id` and a non-NULL
  `pro_until`
- **THEN** the value lands in `pro_until_granted`, and no provider sync clears it

#### Scenario: The derived value is unchanged by the migration

- **WHEN** the migration completes
- **THEN** every account's `pro_until` holds the instant it held before

### Requirement: A store purchase confers Pro on the buying account

The system SHALL set `pro_until_revenuecat` from the configured entitlement in the
subscriber state read from RevenueCat, addressed by the account's `users.id` as
`app_user_id`.

#### Scenario: A purchase entitles until its expiry

- **WHEN** the subscriber holds the configured entitlement with a future `expires_date`
- **THEN** `pro_until_revenuecat` is set to that instant

#### Scenario: A grace period entitles

- **WHEN** the entitlement's `grace_period_expires_date` is later than its `expires_date`
- **THEN** `pro_until_revenuecat` is set to the later of the two, so that a card being
  retried does not cost the subscriber their plan

#### Scenario: An entitlement for something else confers nothing

- **WHEN** the subscriber holds entitlements, none of which is the configured one
- **THEN** `pro_until_revenuecat` is cleared

#### Scenario: A lapsed subscription stops conferring by itself

- **WHEN** the recorded instant passes and nothing renews it
- **THEN** plan resolution returns the free tier without any sweep, worker or further write

### Requirement: An unreadable entitlement confers nothing

The system SHALL treat an entitlement whose reach cannot be determined as conferring no
plan, and SHALL NOT substitute a default, a sentinel or the previous value.

Failing closed costs a subscriber one support message. Failing open gives the product away
and hides that it is doing so.

#### Scenario: An unparseable expiry entitles nobody

- **WHEN** the entitlement is present but neither `expires_date` nor
  `grace_period_expires_date` can be read as an instant
- **THEN** `pro_until_revenuecat` is cleared and the outcome is reported as an error

#### Scenario: A provider that cannot be reached changes nothing

- **WHEN** the read fails because the provider is unreachable or answers an error
- **THEN** no source column is written and the operation is retried later

### Requirement: A non-expiring entitlement is not read as an expired one

The system SHALL treat a present, current entitlement whose `expires_date` is null as
non-expiring rather than as expired.

We sell no such product, which is exactly why this must be right: nobody would notice it
being wrong.

#### Scenario: A null expiry does not downgrade

- **WHEN** the configured entitlement is present with a null `expires_date` and nothing
  marks it as ended
- **THEN** the account is on Pro, and `pro_until_revenuecat` is not set to the zero instant

### Requirement: A bulk pass never manufactures a subscriber

Any pass that can reach accounts in BULK — the reconciler, and the replay of a recorded event
— SHALL read RevenueCat subscriber state only for an account that already has a recorded
RevenueCat event or a non-NULL `pro_until_revenuecat`.

The read endpoint creates the subscriber when the identifier is unknown, so an unconditional
read over the user table would register every account with the provider.

**The rule binds the pass, not the read**, and the distinction is load-bearing. An earlier
draft put the check inside the read itself, which also bound the self-service route below.
A first-time buyer has no recorded event and a NULL column — that is what "first" means — so
all three recovery paths then refused them at once: the replay saw no event, the near-expiry
predicate skips NULL, and the route answered "no subscription" without asking anybody. A
purchase whose first delivery was lost would never have conferred anything, ever.

One authenticated caller asking about their own id is not a bulk pass. They have just bought
something, so the provider's own device SDK created that subscriber before the app reached us
— there is nothing left to manufacture.

#### Scenario: An account that never transacted is not read by the reconciler

- **WHEN** the reconciler passes over an account with no recorded RevenueCat event and a
  NULL `pro_until_revenuecat`
- **THEN** no request is made to the provider for that account

#### Scenario: A caller with no footprint yet is still read

- **WHEN** a signed-in caller with no recorded event and a NULL `pro_until_revenuecat` calls
  the sync route
- **THEN** the provider IS read for them, and their source column is written from the answer

### Requirement: Entitlement is never inferred from the event type

The system SHALL derive `pro_until_revenuecat` from the subscriber state read after an event
arrives, and SHALL NOT branch on the event type to decide what the state became.

Delivery is neither ordered nor guaranteed, and the provider says so. A locally maintained
copy of the provider's state machine is a second source of truth that will disagree with the
first.

#### Scenario: An out-of-order delivery does not corrupt the column

- **WHEN** a renewal event is delivered after the cancellation event that followed it
- **THEN** the resulting `pro_until_revenuecat` matches the provider's current state, not the
  last event's implied meaning

#### Scenario: An unrecognised event type is still applied

- **WHEN** an event of a type the system does not enumerate is delivered
- **THEN** it is recorded, the subscriber is re-read, and the column is derived as usual

### Requirement: A RevenueCat delivery is authenticated before it is recorded

The system SHALL verify the `X-RevenueCat-Webhook-Signature` HMAC-SHA256 over
`"<timestamp>.<raw request body>"` against the configured signing secret, over the bytes
exactly as received, and SHALL reject a signature whose timestamp is outside the configured
freshness window.

#### Scenario: An unsigned delivery is refused

- **WHEN** a delivery arrives with no signature header
- **THEN** it is rejected, nothing is recorded, and no column is written

#### Scenario: A tampered body is refused

- **WHEN** the body does not match the signature
- **THEN** it is rejected and nothing is recorded

#### Scenario: A replayed delivery is refused on age

- **WHEN** a correctly signed delivery arrives with a timestamp outside the freshness window
- **THEN** it is rejected

#### Scenario: Verification precedes parsing

- **WHEN** a delivery is verified
- **THEN** the bytes hashed are the raw request body, not a re-serialisation of the parsed
  event

### Requirement: A received delivery is recorded once

The system SHALL record every authenticated delivery in `billing_events`, unique on
`(provider, event_id)`, and SHALL answer a redelivery as success without recording it twice.

Retries reuse the event id, so a duplicate is the normal case rather than an error.

#### Scenario: A redelivery is idempotent

- **WHEN** the same event id is delivered a second time for the same provider
- **THEN** the response is a success, no second row is written, and the derived plan is
  unchanged

#### Scenario: Event ids do not collide across providers

- **WHEN** a RevenueCat event carries an id equal to a recorded Stripe event's id
- **THEN** both rows exist and both are applied

### Requirement: A caller can force a re-read of their own subscription

The system SHALL expose an authenticated route that re-reads the caller's RevenueCat state
and writes `pro_until_revenuecat`, identifying the caller from the session and taking no
user identifier from the request.

A store purchase completes on the device before the webhook arrives, and the last webhook
retry is 80 minutes after the first. Without this route a paid subscriber waits on a
delivery that may never come.

#### Scenario: A purchase becomes Pro in one round-trip

- **GIVEN** a signed-in caller who has just completed a store purchase
- **WHEN** they call the sync route
- **THEN** the provider is re-read, `pro_until_revenuecat` is written, and the plan surface
  reports Pro

#### Scenario: The route names nobody but the caller

- **WHEN** a request carries a user identifier in its body or query
- **THEN** it is ignored, and the account synced is the session's

#### Scenario: An anonymous caller is refused

- **WHEN** the route is called without a session
- **THEN** it answers unauthorised and makes no request to the provider

#### Scenario: Repeated calls are bounded

- **WHEN** one caller invokes the route more often than the configured allowance
- **THEN** the excess is refused, and the provider is not called for those requests

### Requirement: The plan surface names where Pro came from

`GET /api/v1/me/plan` SHALL carry `pro_source` — one of `stripe`, `revenuecat`, `granted` —
naming the source column whose value equals the derived `pro_until`, resolving a tie in that
order. The field SHALL be absent when the account is on the free plan.

The stores make the origin behavioural. Directing an in-app subscriber to a web page to
cancel violates Apple's rules, and offering an in-app purchase to a Stripe subscriber sells
the same plan twice.

#### Scenario: A store subscriber is identified as one

- **WHEN** the account's Pro comes from `pro_until_revenuecat`
- **THEN** `pro_source` is `revenuecat`

#### Scenario: A free account carries no source

- **WHEN** `pro_until` is NULL or in the past
- **THEN** `pro_source` is absent, as is `pro_until`

#### Scenario: A tie resolves deterministically

- **WHEN** two source columns hold the same instant and it is the derived value
- **THEN** `pro_source` names the earlier of them in the order `stripe`, `revenuecat`,
  `granted`

#### Scenario: The surface answers without the provider

- **WHEN** RevenueCat is unreachable
- **THEN** the plan surface still answers, from the stored columns alone

### Requirement: A reconciler repairs what delivery lost, for each provider

The system SHALL run a scheduled pass that applies events not yet applied and re-reads
subscriptions near their expiry, treating each provider independently and writing only that
provider's column.

Delivery stops for good after five retries, roughly two and a half hours after the event.

**The reconciler cannot recover a FIRST purchase, and saying otherwise would be a lie about
the one case that matters most.** It has two candidate sources and a first purchase is in
neither: the pending-events pass sees only recorded deliveries, and there is none; the
near-expiry pass selects on a non-NULL `pro_until_revenuecat`, and it is NULL until something
writes it. Both are bounded that way on purpose — an unbounded pass over `users` would enrol
every account with the provider.

So for a first purchase whose delivery is lost, the self-service sync route is the ONLY
recovery path, and the client is expected to call it. Every LATER lost delivery — a renewal, a
cancellation, a refund — is recoverable by the reconciler, because by then a footprint exists.

#### Scenario: A lost renewal is recovered

- **GIVEN** an account with a store entitlement whose renewal webhook was never delivered
- **WHEN** the reconciler runs
- **THEN** the account's RevenueCat state is re-read and `pro_until_revenuecat` is corrected

#### Scenario: A first purchase is not the reconciler's to recover

- **GIVEN** a first purchase whose delivery never arrived, so the account has no recorded event
  and a NULL `pro_until_revenuecat`
- **WHEN** the reconciler runs
- **THEN** it does not reach the provider for that account, and the account is recovered by the
  caller's own sync instead

#### Scenario: One provider's outage does not stall the other

- **WHEN** one provider is unreachable during a pass
- **THEN** the other provider's accounts are still reconciled, and the failing pass is
  reported and retried

### Requirement: RevenueCat is inert unless it is configured

The system SHALL mount no RevenueCat route, make no request to RevenueCat and skip its
reconciler pass when its credentials are absent, and SHALL leave Stripe unaffected by that
absence.

#### Scenario: The routes are absent, not broken

- **WHEN** the credentials are unset and a RevenueCat route is requested
- **THEN** the response is not found, distinguishing an unconfigured subsystem from a failing
  one

#### Scenario: Stripe keeps selling

- **WHEN** RevenueCat is unconfigured and a Stripe delivery arrives
- **THEN** it is authenticated, recorded and applied as before

#### Scenario: The worker opens no connection it cannot use

- **WHEN** the reconciler runs with RevenueCat unconfigured
- **THEN** it performs no RevenueCat pass and exits successfully
