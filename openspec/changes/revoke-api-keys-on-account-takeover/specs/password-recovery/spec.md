## MODIFIED Requirements

### Requirement: Password reset by code

The system SHALL set a new password when presented with a valid, unexpired reset code for the
address, SHALL treat a successful reset as proof of email ownership, and SHALL destroy every
credential minted under the old password.

- The new password SHALL satisfy the same rules as registration (8–72 characters).
- The account MAY have had no password before; the reset then sets its first one.
- A successful reset SHALL mark the account's email verified.
- A successful reset SHALL revoke every existing session for that account.
- A successful reset SHALL delete every API key of that account, atomically with the reset.
  A reset by mailed code is how an owner takes an account back after a compromise, and an API
  key authenticates against its own stored hash rather than the account's session generation,
  so revoking the sessions alone would leave whoever knew the old password with programmatic
  access to the recovered account.
- The code SHALL be consumed on success and invalidated after 5 failed attempts.

#### Scenario: Valid code sets the password

- **WHEN** a client POSTs the address, the mailed code, and an 8+ character password to `/api/v1/auth/password/reset`
- **THEN** the system stores the new bcrypt hash, marks the email verified, and responds `200`

#### Scenario: Reset kills existing sessions

- **WHEN** a reset succeeds while another client holds a session cookie for the same account
- **THEN** that client's next authenticated request responds `401`

#### Scenario: Reset kills the account's API keys

- **WHEN** a reset succeeds while an API key exists for the account
- **THEN** the key is deleted and its next request responds `401`

#### Scenario: Wrong or expired code

- **WHEN** a client submits an incorrect or expired code
- **THEN** the system responds `400`, the stored password is unchanged, and the failed attempt counts toward the attempt limit

#### Scenario: Weak new password

- **WHEN** a client submits a valid code with a password shorter than 8 or longer than 72 characters
- **THEN** the system responds `400` and the stored password is unchanged
