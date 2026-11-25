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
token, so third parties cannot forge `/start` updates. The comparison SHALL NOT
be satisfiable by an absent header: a request carrying no secret token SHALL be
rejected regardless of how the server is configured.

#### Scenario: Forged update without the secret is rejected

- **WHEN** a request hits the webhook without the configured secret-token header
- **THEN** the system rejects it and processes no update

#### Scenario: Empty configured secret never admits a request

- **WHEN** the webhook is reached with no secret-token header while the configured secret is the empty string
- **THEN** the system rejects the request and processes no update

### Requirement: Telegram digest sender

The system SHALL send a subscription digest to a linked chat via the Telegram Bot
API as the `telegram` channel's `Notifier` implementation. A send failure SHALL be
reported to the caller so the delivery retry/dead-letter policy applies.

The message SHALL itemize at most ten jobs — the same bound the email channel
applies, so the two channels never disagree about a digest's shape — and SHALL
additionally stop short of that bound when the next job line would carry the message
past Telegram's own message-length limit, because an oversized message is rejected
deterministically and would dead-letter the whole batch. Whatever the listing omits
SHALL be summarized as a "+ N more" tail that links to the digest's matched-jobs
page, `<origin>/my/notifications/<id>/jobs`, falling back to
`<origin>/my/notifications` when the digest carries no notification id.

#### Scenario: Send a digest

- **WHEN** the worker delivers a `telegram` digest for a linked user
- **THEN** the sender posts one message to the user's `chat_id` and reports success or failure to the delivery loop

#### Scenario: A digest of more than ten jobs lists ten and links to the rest

- **WHEN** a digest of 67 matched jobs carrying notification id 42 is rendered for Telegram
- **THEN** the message itemizes 10 jobs and ends with a "+ 57 more" tail linking to `<origin>/my/notifications/42/jobs`

#### Scenario: A message that would overflow is truncated further

- **WHEN** ten job lines would carry the message past Telegram's message-length limit
- **THEN** the message itemizes fewer than ten and the tail's count covers every omitted job

### Requirement: Feature is disabled when unconfigured

The system SHALL treat the Telegram notification feature as disabled unless its bot
credentials **and** its webhook secret are configured, and the SPA SHALL only surface
the linking and subscribe UI when it is enabled. A deployment SHALL NOT be able to
reach a state where the bot is live but the webhook is unauthenticated.

#### Scenario: No bot token configured

- **WHEN** the Telegram bot credentials are absent
- **THEN** the linking endpoints and webhook are inert and the public config reports the feature as disabled

#### Scenario: Bot token without a webhook secret

- **WHEN** the bot token is configured but the webhook secret is absent
- **THEN** the feature stays disabled — the linking endpoints report it off, the webhook responds `404`, and the server logs why

