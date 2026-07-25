## ADDED Requirements

### Requirement: Email ownership is proven by a mailed code

The system SHALL record, per account, whether control of its email address has been proven,
and SHALL prove it either by the account holder entering a code the system mailed to that
address, or by an OAuth provider asserting the address as verified.

- A verification code SHALL be six decimal digits generated from a cryptographically secure
  source, stored only as a hash, and never returned in any response.
- A code SHALL expire 15 minutes after issue and SHALL be single-use.
- At most one outstanding verification code SHALL exist per account; issuing a new one
  replaces the previous one.

#### Scenario: Registration issues a code

- **WHEN** a client registers a new account
- **THEN** the account is recorded as unverified, a six-digit code is mailed to the address, and only the code's hash is stored

#### Scenario: Correct code verifies the account

- **WHEN** the signed-in owner POSTs the mailed code to `/api/v1/auth/verify/confirm`
- **THEN** the system marks the account verified, consumes the code, and responds `200` with the updated user

#### Scenario: Code is single-use

- **WHEN** a code that was already accepted is submitted again
- **THEN** the system responds `400` and the account's verified state is unchanged

#### Scenario: Expired code is refused

- **WHEN** a code older than 15 minutes is submitted
- **THEN** the system responds `400`, the account stays unverified, and the caller is told to request a new code

#### Scenario: OAuth-verified email needs no code

- **WHEN** an account is created from an OAuth sign-in whose provider reports the email as verified
- **THEN** the account is recorded as verified without any code being mailed

### Requirement: Verification attempts and resends are bounded

The system SHALL bound guessing and mail-flooding on the verification endpoints, so a
six-digit code cannot be brute-forced and the address cannot be used as a mail amplifier.

- A code SHALL be invalidated after 5 failed confirmation attempts.
- A resend SHALL be refused while a code issued less than 60 seconds ago is outstanding.

#### Scenario: Too many wrong codes burns the code

- **WHEN** a caller submits a 5th incorrect code for the same outstanding code
- **THEN** the system invalidates that code and every further attempt fails until a new code is requested

#### Scenario: Resend is throttled

- **WHEN** a caller requests a resend less than 60 seconds after the previous code was issued
- **THEN** the system responds `429` and mails nothing

#### Scenario: Resend after the cooldown

- **WHEN** a signed-in, unverified user POSTs to `/api/v1/auth/verify/request` after the cooldown
- **THEN** the system issues and mails a fresh code, replacing any outstanding one

### Requirement: An unverified account is usable but not a merge target

The system SHALL let an unverified account use the product normally, and SHALL NOT treat its
email address as proof of identity: an unverified, password-backed account SHALL NOT silently
absorb an OAuth identity for the same address.

#### Scenario: Unverified user keeps working

- **WHEN** an unverified user calls any authenticated endpoint other than the verification endpoints
- **THEN** the request is served exactly as it is for a verified user

#### Scenario: Verified state is visible to the client

- **WHEN** a client reads `GET /api/v1/auth/me`
- **THEN** the response carries the account's verified state so the SPA can prompt for confirmation

### Requirement: Verification degrades safely without a mail transport

The system SHALL keep registration and sign-in working when no outbound mail transport is
configured, and SHALL NOT pretend a code was delivered.

#### Scenario: Registration without mail configured

- **WHEN** an account is registered while no mail transport is configured
- **THEN** registration still succeeds and the account is created unverified

#### Scenario: Requesting a code without mail configured

- **WHEN** a caller requests a verification code while no mail transport is configured
- **THEN** the system responds `503` and states that email delivery is unavailable
