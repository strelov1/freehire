## MODIFIED Requirements

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
