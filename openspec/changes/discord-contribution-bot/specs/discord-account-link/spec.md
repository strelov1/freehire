## ADDED Requirements

### Requirement: Link a Discord identity to a user account

The system SHALL let an authenticated user link their Discord identity so contributions they
make through the Discord bot are attributed to their account, using a one-time token they carry
into the bot's `/link` command. Because the token cannot be delivered via a deep link the way a
Telegram `/start` link can, the link is completed by the user manually invoking `/link <token>`
in the configured Discord server. The link token SHALL be a short-lived signed token (no
server-side token store) that identifies the user, and SHALL expire.

#### Scenario: Issue a link token

- **WHEN** an authenticated user requests a Discord link
- **THEN** the system returns a token and instructions to run `/link <token>` in the bot's
  channel; the token is a signed, short-TTL credential identifying the user

#### Scenario: Complete the link from /link

- **WHEN** the bot receives `/link <token>` and the token is valid and unexpired
- **THEN** the system stores the invoking Discord user's id against that user, the link becomes
  active, and the bot confirms with a reply visible only to that user

#### Scenario: Expired or invalid token is refused

- **WHEN** the bot receives `/link <token>` with an expired or unverifiable token
- **THEN** no identity is linked and the bot reports the link could not be completed

#### Scenario: Unlink

- **WHEN** the user unlinks Discord
- **THEN** the stored Discord user id is removed and future `/contribute` invocations from that
  identity are treated as unlinked

### Requirement: Interaction webhook is authenticated by Ed25519 signature

The inbound Discord interaction webhook SHALL be the only unauthenticated POST endpoint for this
capability and SHALL reject any request whose body does not verify against the configured
application public key using the signature and timestamp Discord attaches to every interaction,
so third parties cannot forge `/link` or `/contribute` invocations.

#### Scenario: Forged interaction without a valid signature is rejected

- **WHEN** a request hits the interactions webhook with a signature that does not verify against
  the configured public key
- **THEN** the system rejects it and processes no interaction

#### Scenario: Valid Discord PING is acknowledged

- **WHEN** Discord sends the interactions-endpoint verification PING with a valid signature
- **THEN** the system responds with the expected PONG payload

### Requirement: Feature is disabled when unconfigured

The system SHALL treat the Discord bot as disabled unless its bot token, application id, public
key, and guild id are all configured, and the SPA SHALL only surface the linking UI when it is
enabled. A deployment SHALL NOT be able to reach a state where the bot is live but the webhook is
unauthenticated.

#### Scenario: No Discord credentials configured

- **WHEN** the Discord bot credentials are absent
- **THEN** the linking endpoints are inert and the public config reports the feature as disabled

#### Scenario: Partial configuration

- **WHEN** some but not all of the required Discord config values are set
- **THEN** the feature stays disabled — the linking endpoints report it off and the interactions
  webhook responds as not found, and the server logs why
