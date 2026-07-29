## MODIFIED Requirements

### Requirement: External identity account resolution

The system SHALL key external identities by `(provider, provider_user_id)` in
a `user_identities` table referencing `users`, resolving each OAuth sign-in to
exactly one account, and SHALL never merge a provider identity into an account
whose email ownership was never proven without first destroying every credential
that account's previous holder may hold.

- A sign-in matching an existing identity SHALL resolve to its linked user.
- A first-time identity whose **verified** provider email matches an existing
  **verified** account (case-insensitive) SHALL be linked to that account,
  leaving its password and its API keys intact.
- A first-time identity whose **verified** provider email matches an existing
  **unverified**, password-backed account SHALL be linked to that account, and
  the system SHALL clear that account's `password_hash`, mark it verified,
  revoke its existing sessions, and delete its API keys — the address's proven
  owner takes the account and every way the squatter had in stops working.
  Deleting the keys is not covered by revoking the sessions: an API key
  authenticates against its own stored hash and never consults the account's
  session generation, so a key minted before the seizure would otherwise
  outlive it indefinitely.
- The seizure and the key deletion SHALL be atomic with each other, so no path
  can perform one without the other.
- A first-time identity with no matching account SHALL create a new
  passwordless user (`password_hash` NULL), marked verified, plus the identity,
  in one transaction.
- An identity whose provider email is unverified or absent SHALL NOT be linked
  by email and SHALL NOT create an account keyed to that email; the sign-in
  fails.

#### Scenario: Returning OAuth user

- **WHEN** a user signs in via a provider identity that already exists
- **THEN** the system starts a session for the linked user without touching `users` or creating rows

#### Scenario: First OAuth sign-in linking an existing password account

- **WHEN** a user signs in via a new provider identity whose verified email matches an existing **verified** account
- **THEN** the system adds the identity linked to that account and starts a session for it, leaving the password intact

#### Scenario: First OAuth sign-in seizing an unverified account

- **WHEN** a user signs in via a new provider identity whose verified email matches an existing unverified account that has a password
- **THEN** the system links the identity, clears the stored password hash, marks the account verified, bumps its token version, and starts a session — so the previous password and any session minted with it no longer authenticate

#### Scenario: Seizure destroys the previous holder's API keys

- **WHEN** the seized account held an API key, including one minted with no expiry
- **THEN** the key is deleted and its next request responds `401`, so the squatter keeps no bearer access to the account they just lost the password and sessions to

#### Scenario: Seizure touches no other account's keys

- **WHEN** an account is seized while another account holds its own API keys
- **THEN** only the seized account's keys are deleted

#### Scenario: First OAuth sign-in creating a new account

- **WHEN** a user signs in via a new provider identity whose verified email matches no account
- **THEN** the system creates a passwordless, verified user and the identity in one transaction and starts a session

#### Scenario: Unverified provider email never links

- **WHEN** a provider returns an identity whose email is unverified or missing
- **THEN** the system links nothing, creates nothing, and redirects to the SPA with `auth_error`

### Requirement: Global session revocation

The system SHALL let a signed-in user invalidate every session issued for their account,
including the caller's own, by bumping a per-account token version that every authenticated
request is checked against.

- `POST /api/v1/auth/logout-all` SHALL be reachable with a session cookie only.
- The endpoint SHALL bump the account's token version and clear the caller's cookie.
- Password reset, password change, and an OAuth seizure of an unverified account SHALL bump
  the same counter.
- A token-version bump SHALL NOT by itself revoke the account's API keys: a key is a durable
  programmatic credential, not a session, and "sign out everywhere" is the account holder's own
  action. The events that additionally destroy every API key are exactly the two where the
  account changes hands — a password reset by mailed code and an OAuth seizure of an unverified
  account — and they are specified with those flows, not here.

#### Scenario: Sign out everywhere

- **WHEN** a signed-in user POSTs to `/api/v1/auth/logout-all`
- **THEN** the system bumps the account's token version, clears the caller's session cookie, and every other session for that account fails its next authenticated request with `401`

#### Scenario: API keys survive a session revocation

- **WHEN** the account's token version is bumped by `logout-all` or a password change
- **THEN** the account's live API keys continue to authenticate, since they are revoked individually

#### Scenario: Revocation requires a session

- **WHEN** the request presents no session cookie, or authenticates with an API key
- **THEN** the system responds `401` and bumps nothing
