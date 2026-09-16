## Purpose

Greets a new paying subscriber with a personal, repliable email exactly once, the first
time their account becomes entitled to a paid tier — replacing the founder having to
notice and write to each new subscriber by hand.

## ADDED Requirements

### Requirement: One-time welcome email on first payment

The system SHALL send exactly one welcome email to an account the first time its resolved
tier (`plan.TierOf` over `users.pro_until` and `users.ultra_until`) transitions from `free`
to a paying tier (`pro` or `ultra`). The send SHALL be recorded so it is never repeated for
that account, regardless of later renewals, tier changes between `pro` and `ultra`,
cancellation, or a later resubscription.

#### Scenario: Account becomes Pro for the first time

- **WHEN** an account with no prior paid entitlement is resolved to tier `pro` or `ultra`
- **THEN** the system sends it the welcome email once and records that it has been sent

#### Scenario: A renewal re-derives the same paying tier

- **WHEN** a subscription renews and the account's tier is re-derived as still `pro` or
  `ultra`, and the welcome email was already recorded as sent
- **THEN** the system sends no email

#### Scenario: An account cancels and later resubscribes

- **WHEN** an account whose welcome email was already sent lapses to `free` and later
  becomes a paying tier again
- **THEN** the system sends no second welcome email

#### Scenario: An account never pays

- **WHEN** an account's tier is `free`
- **THEN** the system never sends it the welcome email

#### Scenario: Sending fails

- **WHEN** the system attempts the welcome email for a newly-paying account and delivery
  fails
- **THEN** the system does not record the send, so a later run retries it for that account

### Requirement: Personal, repliable welcome email

The welcome email SHALL be sent with a Reply-To header addressed to a human inbox, distinct
from the system's sending address, so a reply from the subscriber reaches a person rather
than an unattended mailbox. The email SHALL include a link to the product's Discord
community and a link to the founder's public profile.

#### Scenario: Subscriber replies to the welcome email

- **WHEN** a subscriber replies to the welcome email they received
- **THEN** the reply is delivered to the configured human Reply-To address, not the
  system's sending address

#### Scenario: Welcome email content

- **WHEN** the welcome email is rendered for delivery
- **THEN** it contains a link to the Discord community and a link to the founder's public
  profile
