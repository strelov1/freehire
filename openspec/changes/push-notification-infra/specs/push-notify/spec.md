## ADDED Requirements

### Requirement: Push token registration

The system SHALL let a signed-in user register a push token for the device
they're currently using, upserted by the token value so a token that changes
owner (a different account signs in on the same device) reassigns to the new
owner rather than duplicating or staying with the previous one.

#### Scenario: New token is registered

- **WHEN** a signed-in user `POST`s a push token and platform (`ios`/`android`) not previously seen
- **THEN** the system creates a row associating that token with the caller

#### Scenario: Re-registering the same token refreshes it

- **WHEN** a signed-in user `POST`s a token that already belongs to them
- **THEN** the system updates `last_seen_at` without creating a duplicate row

#### Scenario: A token changes owner

- **WHEN** a signed-in user `POST`s a token currently registered to a different account
- **THEN** the system reassigns the token's row to the caller, and the previous owner no longer has it among their registered tokens

### Requirement: Push token unregistration

The system SHALL let a signed-in user remove their own registered push
token (e.g. on sign-out), and SHALL cascade-delete a user's push tokens when
their account is deleted.

#### Scenario: Explicit unregistration

- **WHEN** a signed-in user `DELETE`s a token they own
- **THEN** the system removes that row

#### Scenario: Account deletion removes push tokens

- **WHEN** a user's account is deleted
- **THEN** their registered push tokens are removed along with it

### Requirement: Send a push notification via the Expo Push API

The system SHALL send a push message (title, body) to a given token through
the Expo Push API. Expo's immediate response is a per-message ticket, not a
final delivery outcome; a ticket already reporting `DeviceNotRegistered`
SHALL delete that token's row immediately, and any other successfully-sent
ticket SHALL be queued for a later receipt check (see the receipt-polling
requirement below) rather than assumed delivered.

#### Scenario: Successful send

- **WHEN** the notifier sends to a valid, currently-registered Expo push token
- **THEN** the Expo Push API is called with that token, title, and body, the notifier reports success, and the returned ticket id is queued for a later receipt check

#### Scenario: Token already known dead is pruned at send time

- **WHEN** the Expo Push API's send response reports a token as `DeviceNotRegistered`
- **THEN** the system deletes that token's row immediately and does not retry sending to it

#### Scenario: Other send failures are surfaced, token kept

- **WHEN** the Expo Push API reports a send failure other than `DeviceNotRegistered` (e.g. a transient error)
- **THEN** the notifier returns an error and the token row is left in place

### Requirement: Poll Expo receipts to prune tokens that die after sending

The system SHALL periodically check the delivery receipt for each
successfully-sent ticket, once it is old enough for Expo to have an answer,
and SHALL delete a token whose receipt reports `DeviceNotRegistered` — this
is what catches a token going dead for the first time (freshly uninstalled
app, freshly revoked permission), which is not visible in the immediate send
response.

#### Scenario: A freshly-dead token is pruned on receipt check

- **WHEN** a queued ticket's receipt reports `DeviceNotRegistered`
- **THEN** the system deletes that token's row and removes the ticket from the queue

#### Scenario: A delivered ticket is cleared without side effects

- **WHEN** a queued ticket's receipt reports success
- **THEN** the system removes the ticket from the queue and leaves the token row in place

#### Scenario: A ticket not yet old enough is left for a later pass

- **WHEN** a receipt check runs and some queued tickets are younger than the minimum wait
- **THEN** the system only checks tickets old enough to have an answer, leaving the rest queued

### Requirement: Self-test push endpoint

The system SHALL let a signed-in user trigger a test push to their own
registered token(s) only, with no way to target another user's token.

#### Scenario: Test push to own device

- **WHEN** a signed-in user with at least one registered token requests a test push
- **THEN** the system sends a push to each of the caller's own tokens and reports the outcome

#### Scenario: No registered tokens

- **WHEN** a signed-in user with no registered tokens requests a test push
- **THEN** the system reports that there is nothing to send to, without error
