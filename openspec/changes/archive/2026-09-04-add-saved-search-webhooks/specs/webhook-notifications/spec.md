## Purpose

The account-level webhook destination a candidate points at their own tooling, and
the signed HTTP delivery that carries a saved-search digest to it once a saved
search subscribes to the `webhook` channel.

## ADDED Requirements

### Requirement: Creating or rotating the webhook destination

The system SHALL let a signed-in user create or rotate their account's single
webhook destination (URL plus an opaque, high-entropy secret), and SHALL return
the plaintext secret **exactly once**, in the response to that create/rotate
call. The stored secret SHALL be kept in a form the system can recover for
signing later deliveries (not a one-way hash) but SHALL never itself be returned
by any endpoint after that one response. A URL whose scheme is not `http` or
`https` SHALL be rejected and SHALL create or change nothing. Rotating replaces
the destination's secret (and, if provided, its URL) in place — there SHALL be
at most one webhook destination per account.

#### Scenario: First-time creation returns the secret once

- **WHEN** a signed-in user with no existing webhook destination submits a
  `https://` URL
- **THEN** the system creates the destination, enabled, and responds with the
  URL and the plaintext secret
- **AND** no later read of the destination includes that secret or a hash of it

#### Scenario: Rotating replaces the secret

- **WHEN** a signed-in user with an existing webhook destination rotates it
- **THEN** the system generates a new secret, returns it once, and subsequent
  deliveries sign with the new secret only — the previous secret no longer
  validates

#### Scenario: A non-HTTP(S) URL is rejected

- **WHEN** a user submits a URL whose scheme is not `http` or `https`
- **THEN** the system rejects the request and creates or changes nothing

### Requirement: Reading the webhook destination never exposes the secret

The system SHALL let a signed-in user view their webhook destination's
metadata — URL, enabled state, creation time, last successful delivery time,
and disabled time if applicable — and SHALL NOT include the secret or any
derivative of it in that response.

#### Scenario: Viewing the destination omits the secret

- **WHEN** a signed-in user requests their webhook destination after creating it
- **THEN** the response includes the URL and status fields but no secret

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

### Requirement: Delivered payloads are HMAC-signed

The system SHALL sign every webhook delivery's request body with HMAC-SHA256
keyed by the destination's current secret, and SHALL carry the signature in a
request header, so the receiver can verify the request originated from this
system and was not altered in transit.

#### Scenario: Signature matches a receiver's own computation

- **WHEN** a receiver recomputes HMAC-SHA256 over the exact bytes of a received
  request body using the secret it was given at creation or rotation
- **THEN** the recomputed value equals the signature header on that request

#### Scenario: A rotated secret changes future signatures only

- **WHEN** a destination's secret is rotated
- **THEN** deliveries sent after the rotation are signed with the new secret,
  and a receiver still validating with the old secret can no longer verify them

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
