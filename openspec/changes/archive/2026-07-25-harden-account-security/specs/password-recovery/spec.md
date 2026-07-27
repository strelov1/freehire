## ADDED Requirements

### Requirement: Forgotten-password code request

The system SHALL let anyone request a password-reset code for an email address, and SHALL
respond identically whether or not an account exists for that address, so the endpoint is not
an account-enumeration oracle.

- A reset code SHALL be six decimal digits from a cryptographically secure source, stored
  only as a hash, single-use, and valid for 15 minutes.
- A code SHALL be mailed whenever the address has an account, including a passwordless
  (OAuth-only) one — its owner may use this flow to set a first password and gain a
  second way in. Receiving the code proves control of the address, the same proof the
  provider offers, so refusing would strand the user without buying safety.
- Requests SHALL be rate-limited per address and per client IP.

#### Scenario: Known address receives a code

- **WHEN** a client POSTs a registered, password-backed address to `/api/v1/auth/password/forgot`
- **THEN** the system mails a six-digit code and responds `202` with no account details

#### Scenario: Unknown address is indistinguishable

- **WHEN** a client POSTs an address that has no account
- **THEN** the system responds `202` with the same body and timing class as for a known address, and mails nothing

#### Scenario: OAuth-only account may set a first password

- **WHEN** a client requests a reset for an address whose account has no password
- **THEN** the system responds `202` and mails a code, and completing the reset gives that account its first password

### Requirement: Password reset by code

The system SHALL set a new password when presented with a valid, unexpired reset code for the
address, and SHALL treat a successful reset as proof of email ownership.

- The new password SHALL satisfy the same rules as registration (8–72 characters).
- The account MAY have had no password before; the reset then sets its first one.
- A successful reset SHALL mark the account's email verified.
- A successful reset SHALL revoke every existing session for that account.
- The code SHALL be consumed on success and invalidated after 5 failed attempts.

#### Scenario: Valid code sets the password

- **WHEN** a client POSTs the address, the mailed code, and an 8+ character password to `/api/v1/auth/password/reset`
- **THEN** the system stores the new bcrypt hash, marks the email verified, and responds `200`

#### Scenario: Reset kills existing sessions

- **WHEN** a reset succeeds while another client holds a session cookie for the same account
- **THEN** that client's next authenticated request responds `401`

#### Scenario: Wrong or expired code

- **WHEN** a client submits an incorrect or expired code
- **THEN** the system responds `400`, the stored password is unchanged, and the failed attempt counts toward the attempt limit

#### Scenario: Weak new password

- **WHEN** a client submits a valid code with a password shorter than 8 or longer than 72 characters
- **THEN** the system responds `400` and the stored password is unchanged

### Requirement: Password change while signed in

The system SHALL let a signed-in user change their password by presenting the current one,
and SHALL keep the caller signed in while revoking every other session.

- The endpoint SHALL be reachable with a session cookie only, never with an API key.
- An account with no password SHALL be able to set one only through the reset flow, not here.
- The caller's own session SHALL be re-issued so the change does not sign them out.

#### Scenario: Correct current password changes it

- **WHEN** a signed-in user POSTs the correct current password and a valid new one to `/api/v1/me/password`
- **THEN** the system stores the new hash, responds `200`, and sets a fresh session cookie for the caller

#### Scenario: Other sessions are revoked

- **WHEN** the change succeeds while the same account has a session on another device
- **THEN** that other session's next authenticated request responds `401`

#### Scenario: Wrong current password

- **WHEN** the submitted current password does not match
- **THEN** the system responds `401`, changes nothing, and revokes no session

#### Scenario: API key cannot change the password

- **WHEN** the request authenticates with an API key instead of the session cookie
- **THEN** the system responds `401` and changes nothing
