# webhook-notifications Specification

## Purpose

The account-level webhook destination a candidate points at their own tooling, and
the plain HTTP delivery that carries a saved-search digest to it once a saved
search subscribes to the `webhook` channel.

## Requirements

### Requirement: Creating or updating the webhook destination

The system SHALL let a signed-in user create their account's single webhook
destination (a URL), or update its URL if one already exists. A URL whose
scheme is not `http` or `https` SHALL be rejected and SHALL create or change
nothing. There SHALL be at most one webhook destination per account.

#### Scenario: First-time creation

- **WHEN** a signed-in user with no existing webhook destination submits a
  `https://` URL
- **THEN** the system creates the destination, enabled, and responds with the
  URL

#### Scenario: Updating replaces the URL

- **WHEN** a signed-in user with an existing webhook destination submits a
  different URL
- **THEN** the destination's URL is replaced, and subsequent deliveries go to
  the new URL only

#### Scenario: A non-HTTP(S) URL is rejected

- **WHEN** a user submits a URL whose scheme is not `http` or `https`
- **THEN** the system rejects the request and creates or changes nothing

### Requirement: Enabling, disabling, and deleting the destination

The system SHALL let a signed-in user disable, re-enable, or delete their
webhook destination. While disabled or deleted, a saved search subscribed to
the `webhook` channel SHALL be softly skipped at delivery (its matches stay
pending, no delivery attempt is counted) rather than treated as a failure, the
same way an unconfigured channel is skipped today.

#### Scenario: Disabling stops deliveries without losing pending matches

- **WHEN** a user disables their enabled webhook destination
- **THEN** subsequent delivery passes softly skip webhook subscriptions for
  that account, and their pending matches remain pending rather than being
  dead-lettered

#### Scenario: Re-enabling resumes delivery

- **WHEN** a user re-enables a previously disabled webhook destination
- **THEN** the next delivery pass delivers that account's pending webhook
  matches normally

### Requirement: Delivery is SSRF-safe

The system SHALL deliver only to `http`/`https` URLs and SHALL use an
SSRF-guarded transport for every delivery attempt, so a destination that
resolves to a private, loopback, or link-local address — whether at creation
time or after a later DNS change, including across a redirect — is refused
rather than delivered.

#### Scenario: A destination that resolves to a private address is refused

- **WHEN** a delivery attempt's URL resolves to a private or loopback address
- **THEN** the system refuses to send the request and the attempt is treated as
  a failed delivery

### Requirement: A Gone response disables the destination immediately

The system SHALL treat an HTTP `410 Gone` response from the destination as a
definitive signal that the endpoint is intentionally retired, and SHALL disable
the webhook destination immediately on receiving it — without waiting for the
match's normal attempt/dead-letter bound, and without counting the delivery as
a failed attempt against that match.

#### Scenario: A 410 response disables the destination

- **WHEN** a delivery attempt receives an HTTP `410 Gone` response
- **THEN** the system disables the account's webhook destination immediately,
  releases the match without marking it notified or counting a failed attempt,
  and no further delivery is attempted until the user re-enables the
  destination

#### Scenario: Other error responses do not disable the destination

- **WHEN** a delivery attempt receives a response other than `410` that is not
  a success (for example a timeout, a `5xx`, or a `4xx` other than `410`)
- **THEN** the webhook destination stays enabled and the match follows the
  existing per-match retry/dead-letter path
