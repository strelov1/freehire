# telegram-notify Specification

## Purpose
TBD - created by syncing change filter-subscriptions. Update Purpose after archive.
## Requirements
### Requirement: Link a Telegram chat to a user account

The system SHALL let an authenticated user link their Telegram chat so the bot
can message them, using a deep-link token they carry into the bot. Because a bot
cannot initiate contact, the link is completed by an inbound message from the
user. The link token SHALL be a short-lived signed token (no server-side token
store) that identifies the user, and SHALL expire.

#### Scenario: Issue a deep link

- **WHEN** an authenticated user requests a Telegram link
- **THEN** the system returns a `t.me/<bot>?start=<token>` URL whose token is a signed, short-TTL credential identifying the user

#### Scenario: Complete the link from /start

- **WHEN** the bot receives `/start <token>` and the token is valid and unexpired
- **THEN** the system stores the user's `chat_id`, the link becomes active, and the bot confirms to the user

#### Scenario: Expired or invalid token is refused

- **WHEN** the bot receives `/start <token>` with an expired or unverifiable token
- **THEN** no chat is linked and the bot reports the link could not be completed

#### Scenario: Unlink

- **WHEN** the user unlinks Telegram
- **THEN** the stored `chat_id` is removed and Telegram deliveries for that user stop

### Requirement: Inbound webhook is authenticated by a shared secret

The inbound Telegram webhook SHALL be the only unauthenticated POST endpoint and
SHALL reject any request that does not present the configured Telegram secret
token, so third parties cannot forge `/start` updates.

#### Scenario: Forged update without the secret is rejected

- **WHEN** a request hits the webhook without the configured secret-token header
- **THEN** the system rejects it and processes no update

### Requirement: Telegram digest sender

The system SHALL send a subscription digest to a linked chat via the Telegram Bot
API as the `telegram` channel's `Notifier` implementation. A send failure SHALL be
reported to the caller so the delivery retry/dead-letter policy applies.

#### Scenario: Send a digest

- **WHEN** the worker delivers a `telegram` digest for a linked user
- **THEN** the sender posts one message to the user's `chat_id` and reports success or failure to the delivery loop

### Requirement: Feature is disabled when unconfigured

The system SHALL treat the Telegram notification feature as disabled when its bot
credentials are not configured, and the SPA SHALL only surface the linking and
subscribe UI when it is enabled.

#### Scenario: No bot token configured

- **WHEN** the Telegram bot credentials are absent
- **THEN** the linking endpoints and webhook are inert and the public config reports the feature as disabled
