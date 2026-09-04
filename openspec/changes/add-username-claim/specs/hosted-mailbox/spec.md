## MODIFIED Requirements

### Requirement: Claim a hosted mailbox address

The system SHALL let a signed-in user claim a mailbox address on the receiving
domain (`<username>@<MAIL_DOMAIN>`), where `<username>` is the caller's account
username (allocated by default if the caller does not already have one).
Claiming MUST be idempotent — a user who already has a mailbox gets the same
address back, never a second one. The mailbox address always tracks the
caller's current account username; it is no longer allocated or suffixed
independently by the mailbox feature.

#### Scenario: First claim allocates an address from the account's username

- **WHEN** a signed-in user without a mailbox claims one
- **THEN** the system resolves (or lazily allocates) the caller's account username, stores `<username>@<MAIL_DOMAIN>` against the user, and returns it

#### Scenario: Re-claim returns the same address

- **WHEN** a user who already has a mailbox claims again
- **THEN** the same address is returned and no second mailbox is created

#### Scenario: A username change updates the mailbox address

- **WHEN** a user with a claimed mailbox changes their account username
- **THEN** the mailbox address reflects the new username on next access, without a separate mailbox-specific claim step
