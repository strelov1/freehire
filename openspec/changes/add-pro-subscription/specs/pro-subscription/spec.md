## ADDED Requirements

### Requirement: A signed-in user can reach a checkout for the Pro plan

The system SHALL provide an authenticated endpoint that returns a checkout URL for the
Pro plan on the configured billing provider's hosted paywall. The URL SHALL carry the
caller's own `users.id` as the provider's `app_user_id`. The system SHALL NOT accept,
transmit or store card details, and SHALL NOT render a payment form of its own.

#### Scenario: An authenticated user requests checkout

- **WHEN** a signed-in user requests a Pro checkout
- **THEN** a URL to the provider's hosted paywall is returned, carrying that user's own
  identifier, and no card data is handled by the system

#### Scenario: An anonymous caller requests checkout

- **WHEN** a caller with no session requests a Pro checkout
- **THEN** the request is refused as unauthenticated and no URL is issued, so that a
  purchase can never be attributed to an anonymous provider identifier

#### Scenario: The identifier is not supplied by the client

- **WHEN** a checkout URL is issued
- **THEN** the identifier it carries is taken from the authenticated session on the
  server, and a user-supplied identifier in the request is ignored

### Requirement: The plan column is derived from the provider's current state

The system SHALL set `users.pro_until` from the subscription state read from the billing
provider at the moment of the write, and SHALL NOT accumulate it from the sequence of
events received. Applying the same provider state twice SHALL leave the column unchanged.

#### Scenario: An active entitlement

- **WHEN** the provider reports an entitlement conferring Pro with an expiry in the future
- **THEN** `users.pro_until` is set to that expiry

#### Scenario: More than one entitlement conferring Pro

- **WHEN** the provider reports several entitlements conferring Pro with different expiries
- **THEN** `users.pro_until` is set to the latest of them

#### Scenario: No entitlement conferring Pro

- **WHEN** the provider reports no entitlement conferring Pro, whether because the
  subscription lapsed, was refunded, or was transferred to another account
- **THEN** `users.pro_until` is cleared or set to the expiry the provider reports, and the
  user resolves to the free plan without any sweep, cron or expiry job running

#### Scenario: An entitlement in its grace period

- **WHEN** the provider reports an entitlement whose payment failed but whose grace period
  extends beyond its expiry
- **THEN** `users.pro_until` is set to the end of the grace period, so that a card that
  needs renewing does not remove access the subscriber is still entitled to

#### Scenario: An entitlement that does not expire

- **WHEN** the provider reports an entitlement conferring Pro with no expiry at all
- **THEN** the user resolves to the pro plan, and the absent expiry is NOT read as an
  expiry in the past

#### Scenario: The same state applied twice

- **WHEN** the same provider state is applied to the same user a second time
- **THEN** `users.pro_until` holds the same value it held after the first application

#### Scenario: A user the provider has never seen

- **WHEN** provider state is read
- **THEN** it is read only for users who already have a recorded billing event or a
  non-NULL `pro_until`, so that reading state can never bring a subscriber record into
  existence at the provider for a user who has never transacted

### Requirement: The upgrade offer is absent when there is nothing to sell

The system SHALL show an upgrade entry point only when a checkout can actually be issued.
When billing is unconfigured, or no paywall is configured, the surface SHALL omit the
entry point rather than render it as failing.

#### Scenario: A free-plan user on a deployment with billing configured

- **WHEN** a free-plan user opens the plan surface
- **THEN** an upgrade entry point is shown, leading to the checkout URL issued for them

#### Scenario: A deployment with no billing

- **WHEN** a free-plan user opens the plan surface and billing is unconfigured
- **THEN** no upgrade entry point is shown, and no error is presented

#### Scenario: A subscriber sees when their plan runs to

- **WHEN** a pro-plan user opens the plan surface
- **THEN** it states the date the subscription runs until, read from stored state without
  calling the billing provider

### Requirement: Where to cancel comes from the provider

The system SHALL obtain the subscription management destination from the billing provider
rather than composing one, and SHALL request it only when a surface is about to show it.
When it cannot be obtained, the surface SHALL omit the link while still stating that
deleting an account does not cancel a subscription.

#### Scenario: A subscriber opens the delete-account surface

- **WHEN** a member with a subscription opens the delete-account surface
- **THEN** the management destination is requested from the provider and offered as a link

#### Scenario: The provider cannot be reached

- **WHEN** the management destination cannot be obtained
- **THEN** the surface still states that deletion does not cancel the subscription, and
  simply shows no link

#### Scenario: The destination is not composed locally

- **WHEN** a management destination is shown
- **THEN** it is the value the provider reported for that subscriber, not a URL derived
  from configuration

### Requirement: The plan column has exactly two writers

The system SHALL write `users.pro_until` only from the webhook handler and the
reconciler. No other request path, worker or feature SHALL write it. A value set directly
in the database for support SHALL remain safe, because the column expires by itself and
requires no action to withdraw.

#### Scenario: A metered feature runs

- **WHEN** any metered feature consumes an allowance
- **THEN** `users.pro_until` is unchanged by that action

#### Scenario: A support-set value lapses

- **WHEN** `users.pro_until` is set directly for support and nobody removes it
- **THEN** the account resolves to the free plan from the instant that timestamp passes,
  with no sweep or scheduled job involved

### Requirement: Deleting an account erases its billing records

Deleting an account SHALL erase the billing events recorded against it, together with the
rest of that member's data. The system SHALL NOT cancel or refund the member's
subscription at the provider on their behalf, and the deletion surface SHALL say so
before the member confirms.

#### Scenario: A subscriber deletes their account

- **WHEN** an account with recorded billing events is deleted
- **THEN** no billing event keyed to that user remains

#### Scenario: The member is told what deletion does not do

- **WHEN** a member with an active subscription opens the delete-account surface
- **THEN** it states that deleting the account does not cancel the subscription, and links
  to the management URL the provider itself reports for that subscriber rather than to a
  destination the system composed

### Requirement: A received webhook is recorded before it is applied

The system SHALL record every webhook event it accepts in an append-only store, keyed
uniquely by provider and provider event identifier, and SHALL acknowledge the delivery
with HTTP 200 once the event is durably recorded — before any attempt to apply it.

#### Scenario: An event is delivered

- **WHEN** an authenticated webhook delivery arrives
- **THEN** the event is stored with its payload and acknowledged with 200, whether or not
  the resulting plan change can be applied at that moment

#### Scenario: The same event is delivered twice

- **WHEN** an event with an identifier already recorded is delivered again
- **THEN** the delivery is acknowledged with 200 and produces no second record and no
  second application

#### Scenario: The provider cannot be reached while applying

- **WHEN** an event is recorded but the provider cannot be read to derive the new state
- **THEN** the delivery is still acknowledged with 200, the event remains marked
  unprocessed, and the plan change is applied later by the reconciler

#### Scenario: The event cannot be recorded

- **WHEN** the event cannot be durably recorded at all
- **THEN** the delivery is NOT acknowledged, so that the provider redelivers it — an
  acknowledgement is a claim that the event is stored, and claiming it falsely is the one
  way an event is lost for good

### Requirement: Entitlement is never inferred from the event type

The system SHALL derive a user's plan only from the provider's reported subscription
state, and SHALL NOT branch on the webhook's event type to grant, extend or revoke Pro.

#### Scenario: Events arrive out of order

- **WHEN** two events for one user are delivered in an order other than the order they
  occurred in
- **THEN** the resulting `users.pro_until` matches the provider's current state, and does
  not depend on the order the events arrived in

#### Scenario: An event type the system does not recognise

- **WHEN** an event of an unrecognised type is delivered for a known user
- **THEN** it is recorded and triggers the same re-read of provider state as any other
  event, rather than being discarded or specially interpreted

### Requirement: The webhook authenticates every delivery before recording it

The system SHALL verify a webhook delivery's signature against the configured signing
secret before recording the event or changing any plan, and SHALL perform the comparison
in constant time. The signature SHALL be computed over the request body exactly as
received, before any parsing. The system SHALL reject a delivery whose signed timestamp
lies outside a bounded freshness window.

#### Scenario: A delivery with no or wrong signature

- **WHEN** a webhook delivery arrives unsigned, or signed with a wrong secret
- **THEN** it is rejected, no event is recorded, and no plan is changed

#### Scenario: A valid delivery replayed later

- **WHEN** a correctly signed delivery is replayed after the freshness window has passed
- **THEN** it is rejected, so that a captured delivery is not a reusable credential

#### Scenario: The body is verified as received

- **WHEN** a delivery's body is verified
- **THEN** the bytes verified are the bytes received, not a re-serialisation of a parsed
  value, so that a valid delivery is never rejected because parsing changed it

### Requirement: A reconciler repairs what webhook delivery lost

The system SHALL run a scheduled reconciler that applies recorded but unprocessed events
and re-reads provider state for users whose subscription is near its recorded expiry.
Running it SHALL be safe at any time and SHALL be idempotent.

#### Scenario: A webhook was never delivered

- **WHEN** a subscription renews at the provider but no webhook for it was ever received,
  and the user has a recorded event or a non-NULL `pro_until`
- **THEN** the reconciler re-reads the provider state and `users.pro_until` is corrected
  within its scheduled interval

#### Scenario: An event could not be applied when received

- **WHEN** an unprocessed event is present
- **THEN** the reconciler applies it and marks it processed

#### Scenario: The reconciler runs with nothing to do

- **WHEN** the reconciler runs and every event is processed and no subscription is near
  expiry
- **THEN** it writes nothing and exits successfully

### Requirement: Billing is inert unless it is configured

The system SHALL treat billing as disabled when its provider credentials are absent.
Every billing route SHALL then answer as though it does not exist, and the reconciler
SHALL exit successfully without opening a database connection. Absent configuration SHALL
NOT fail application startup.

#### Scenario: A deployment with no billing configuration

- **WHEN** the application starts with no billing provider credentials configured
- **THEN** it starts normally, every billing route returns 404, and every other feature
  behaves exactly as it does with billing configured

#### Scenario: The reconciler with no billing configuration

- **WHEN** the reconciler runs with no billing provider credentials configured
- **THEN** it exits successfully having opened no database connection

### Requirement: Plan resolution remains independent of the provider

The system SHALL NOT call the billing provider on any request path that resolves a user's
plan or meters an action. A provider that is slow, failing or unreachable SHALL NOT delay
or fail any metered action.

#### Scenario: The provider is unreachable

- **WHEN** the billing provider cannot be reached
- **THEN** every metered feature continues to work, evaluated against the plan already
  recorded in `users.pro_until`
