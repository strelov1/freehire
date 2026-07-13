## ADDED Requirements

### Requirement: Claim a hosted mailbox address

The system SHALL let a signed-in user claim a mailbox address on the receiving
domain (`<handle>@<MAIL_DOMAIN>`), derived from their email and made unique by a
numeric suffix on collision. Claiming MUST be idempotent — a user who already has a
mailbox gets the same address back, never a second one.

#### Scenario: First claim allocates an address

- **WHEN** a signed-in user without a mailbox claims one
- **THEN** the system allocates `<handle>@<MAIL_DOMAIN>`, stores it against the user, and returns it

#### Scenario: Re-claim returns the same address

- **WHEN** a user who already has a mailbox claims again
- **THEN** the same address is returned and no second mailbox is created

#### Scenario: Handle collision is suffixed

- **WHEN** the derived handle is already taken by another user
- **THEN** the new mailbox gets the smallest free numeric suffix (`handle-2`, `handle-3`, …)

### Requirement: Read mailbox status

The system SHALL expose the caller's mailbox address, or that they have none, and
report whether the hosted-mailbox feature is available (configured).

#### Scenario: User has a mailbox

- **WHEN** a signed-in user with a mailbox requests their mailbox status
- **THEN** the response returns their address

#### Scenario: User has no mailbox

- **WHEN** a signed-in user without a mailbox requests their mailbox status
- **THEN** the response reports no address

#### Scenario: Feature unconfigured

- **WHEN** the hosted-mailbox feature is not configured (no `MAIL_DOMAIN`)
- **THEN** the status reports the feature unavailable and the claim action is not offered

### Requirement: Ingest received mail under the owning user

The system SHALL receive mail addressed to a hosted mailbox, parse it, resolve the
recipient to the owning user, and store the message in the unified mail store as a
`hosted`-source message. Storage MUST be idempotent by the message's RFC `Message-ID`
(synthesized from a stable key when the header is absent), and MUST be best-effort per
message — an unparseable body or an unknown recipient is dropped, not fatal.

#### Scenario: Mail to a known mailbox is stored

- **WHEN** the ingest worker receives mail addressed to an allocated mailbox
- **THEN** it stores a `hosted` message under that mailbox's user with the from, subject, bodies, and received time

#### Scenario: Redelivery is idempotent

- **WHEN** the same message is delivered twice
- **THEN** only one stored message exists (dedup on the Message-ID)

#### Scenario: Unknown recipient is dropped

- **WHEN** mail arrives for an address with no mailbox
- **THEN** the worker drops it without error and processes the rest of the batch

#### Scenario: Transient store failure is retried

- **WHEN** storing a received message fails transiently
- **THEN** the message is not acknowledged so the transport redelivers it

### Requirement: Release a hosted mailbox

The system SHALL let a user release their mailbox, which removes the address and
purges that mailbox's received mail, leaving mail from other sources untouched.

#### Scenario: Release purges hosted mail only

- **WHEN** a user releases their mailbox
- **THEN** the address and its `hosted` messages are removed and the user's Gmail-sourced mail is unaffected
