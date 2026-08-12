## MODIFIED Requirements

### Requirement: One-shot delivery

A reminder SHALL fire exactly once at or after its scheduled fire time and then be marked
delivered. A due reminder SHALL be delivered as a message over each channel in the rule's
channel set for which the user has a usable destination, reusing the existing notification
delivery engine. For the `push` channel, a usable destination is at least one currently
registered device; delivery fans out to every registered device and counts as sent for that
channel as long as at least one device receives it. Delivery SHALL be idempotent under
worker retries: a reminder already marked delivered is never sent again.

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

#### Scenario: Push channel with no registered device

- **WHEN** a reminder's rule includes `push` but the user has no currently registered device
- **THEN** that channel is skipped without failing the reminder, and remaining channels
  still deliver

#### Scenario: Push channel fans out to every registered device

- **WHEN** a reminder's rule includes `push` and the user has two registered devices
- **THEN** both devices receive the reminder, and the channel counts as delivered as long
  as at least one device received it

#### Scenario: Not yet due

- **WHEN** the worker runs before a reminder's fire time
- **THEN** the reminder is left pending and nothing is sent

## ADDED Requirements

### Requirement: Push reminder content and deep link

A reminder delivered over `push` SHALL render as a short title and body naming the saved
job and its company, distinct from the longer Telegram/email copy, and SHALL carry that
job's slug as deep-link data so the mobile app can open it directly — a reminder always
concerns exactly one job, so a deep link is always available.

#### Scenario: Push reminder carries a deep link

- **WHEN** a reminder is delivered over `push`
- **THEN** the push's data payload includes the saved job's slug
