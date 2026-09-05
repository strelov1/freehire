## ADDED Requirements

### Requirement: Three tiers, resolved from what the account holds

The system SHALL recognise three tiers — free, pro and ultra. A tier SHALL be resolved from
the account's own entitlement columns and from nothing else, with no call to a payment
provider on the path.

#### Scenario: An account holding nothing

- **WHEN** an account has neither a pro nor an ultra entitlement reaching into the future
- **THEN** its tier is free

#### Scenario: An account holding only Pro

- **WHEN** an account's pro entitlement reaches into the future and its ultra entitlement
  does not
- **THEN** its tier is pro

#### Scenario: An account holding Ultra

- **WHEN** an account's ultra entitlement reaches into the future
- **THEN** its tier is ultra, whatever its pro entitlement says

#### Scenario: Both subscriptions at once

- **WHEN** an account holds a live Pro subscription and a live Ultra subscription
- **THEN** its tier is ultra — the better of the two, so that buying the more expensive plan
  can never give somebody less

#### Scenario: An entitlement that has just lapsed

- **WHEN** an account's ultra entitlement is exactly now or earlier, and its pro entitlement
  reaches into the future
- **THEN** its tier is pro

#### Scenario: Resolving a tier reaches no provider

- **WHEN** a tier is resolved while the payment provider is unreachable
- **THEN** the tier still resolves, because it is read from stored columns

### Requirement: Each tier's entitlement has one writer per origin

An ultra entitlement SHALL be recorded in one source column per origin — the web provider,
the store provider, and a manual grant — and the effective `ultra_until` SHALL be derived by
the schema as the furthest of them. Assigning the derived column directly SHALL be refused.

#### Scenario: A provider reports no subscription

- **WHEN** the web provider reports that an account has no Ultra subscription
- **THEN** only that provider's own source column is cleared, and an ultra entitlement from
  another origin still stands

#### Scenario: Writing the derived column

- **WHEN** an `UPDATE` assigns `ultra_until` directly
- **THEN** the write is refused rather than silently revoking somebody's plan

### Requirement: A provider answers for every tier it can sell

The provider seam SHALL carry how far a provider's entitlement reaches FOR EACH TIER, and a
provider SHALL write every tier column it owns on every sync — including the ones it reports
nothing for.

#### Scenario: The web provider sees an Ultra subscription

- **WHEN** an account's subscription is for a price in the Ultra price list
- **THEN** that provider's ultra column reaches the subscription's period end, and its pro
  column reports nothing

#### Scenario: A subscription is cancelled

- **WHEN** an account's Ultra subscription ends and it holds nothing else
- **THEN** the provider's ultra column is cleared on the next sync, because a provider that
  reported a tier must be able to stop reporting it

#### Scenario: A provider that sells no Ultra

- **WHEN** the store provider syncs an account
- **THEN** it reports nothing for ultra and writes that, rather than leaving the column
  untouched

### Requirement: Which tier a purchase confers is decided by price list

The tier a subscription confers SHALL be decided by which configured price list its price
appears in. A subscription for a price in no list SHALL confer nothing.

#### Scenario: A price nobody configured

- **WHEN** an account's subscription is for a price in neither list
- **THEN** it confers no tier at all

#### Scenario: The Ultra list is unset

- **WHEN** the deployment names no Ultra prices
- **THEN** no account is ever resolved to ultra, and pro and free behave exactly as before

### Requirement: Every tier's daily allowance is configured per feature

Each metered feature SHALL carry a daily allowance for every tier. The ultra tier SHALL be
unlimited wherever it is offered, with a fair-use figure behind it in the same way pro is.

#### Scenario: An ultra account reads its allowances

- **WHEN** an account on ultra asks what it may do today
- **THEN** each feature reports unlimited, with a fair-use ceiling behind it

#### Scenario: A feature nobody configured

- **WHEN** an allowance is asked for a feature with no configuration
- **THEN** the answer is a zero allowance rather than an unbounded one
