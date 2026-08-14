## MODIFIED Requirements

### Requirement: One-shot delivery

A reminder SHALL fire exactly once at or after its scheduled fire time and then be marked
delivered. A due reminder SHALL be delivered as a message over each channel in the rule's
channel set for which the user has a usable destination, reusing the existing notification
delivery engine. Delivery SHALL be idempotent under worker retries: a reminder already
marked delivered is never sent again. If the account's local time falls inside its
configured quiet-hours window when a reminder becomes due, delivery SHALL be deferred to a
later worker pass rather than sent or dropped.

#### Scenario: Due reminder is delivered once

- **WHEN** the reminder worker runs after a reminder's fire time has passed
- **THEN** the user receives one reminder message per configured channel with a usable
  destination
- **AND** the reminder is marked delivered

#### Scenario: Worker re-run does not resend

- **WHEN** the worker runs again after a reminder was already delivered
- **THEN** no additional message is sent for that reminder

#### Scenario: Channel has no destination

- **WHEN** a reminder's rule includes `telegram` but the user has not linked Telegram
- **THEN** that channel is skipped without failing the reminder, and remaining channels
  still deliver

#### Scenario: Not yet due

- **WHEN** the worker runs before a reminder's fire time
- **THEN** the reminder is left pending and nothing is sent

#### Scenario: Due reminder deferred during quiet hours

- **WHEN** a reminder becomes due while the account's local time is inside
  its configured quiet-hours window
- **THEN** delivery is deferred (the claim is released, not marked
  delivered or failed) and retried on a later pass
