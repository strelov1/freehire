## Purpose

Governs how a signed-in user buys, upgrades, downgrades, and views the state of a paid subscription (Pro/Ultra) against the payment provider, so a tier change never results in more than one subscription billing the same customer.

## ADDED Requirements

### Requirement: Upgrading or downgrading modifies the existing subscription
When a customer who already has exactly one active, entitling subscription requests a different tier, the system SHALL change that existing subscription's price in place (with proration) rather than starting a new, separate subscription. The customer SHALL end this action with exactly one active entitling subscription.

#### Scenario: Customer with an active Pro subscription upgrades to Ultra
- **WHEN** a customer with an active Pro subscription requests the Ultra tier
- **THEN** the system updates the existing subscription's price to the Ultra price with proration
- **AND** no new subscription is created for that customer
- **AND** the customer has exactly one active entitling subscription afterward

#### Scenario: Customer with an active Ultra subscription downgrades to Pro
- **WHEN** a customer with an active Ultra subscription requests the Pro tier
- **THEN** the system updates the existing subscription's price to the Pro price with proration
- **AND** no new subscription is created for that customer

### Requirement: A customer already holding more than one entitling subscription is not retroactively fixed
When a customer is found to already have more than one active entitling subscription (a state this system no longer creates going forward, but does not undo for an account it already happened to), a further tier change SHALL modify only the best-entitling one — never open an additional subscription, and never guess at reconciling the others.

#### Scenario: Customer already has two concurrent entitling subscriptions and requests a further tier change
- **WHEN** a customer who already holds two active entitling subscriptions (at different tiers) requests a third tier
- **THEN** the system changes the best-entitling subscription's price in place
- **AND** the other, already-duplicated subscription is left untouched
- **AND** no new subscription is created for that customer

### Requirement: A customer's first subscription is created via checkout
When a customer has no active entitling subscription, requesting a tier SHALL create a new subscription through the payment provider's hosted checkout, as before.

#### Scenario: Customer with no active subscription buys Pro
- **WHEN** a customer with no active entitling subscription requests the Pro tier
- **THEN** the system creates a new subscription via a hosted checkout session
- **AND** the resulting subscription is billed at the Pro price

### Requirement: The billing overview reports more than one entitling subscription
If a customer is found to have more than one active entitling subscription at the payment provider, the billing overview SHALL report this state explicitly instead of silently presenting only the best-entitling one.

#### Scenario: Customer already has two concurrent subscriptions
- **WHEN** a customer's billing overview is requested and the payment provider reports more than one active entitling subscription for that customer
- **THEN** the overview response indicates that more than one entitling subscription is active
- **AND** the overview still reports the best-entitling tier for access-control purposes
