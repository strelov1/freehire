## MODIFIED Requirements

### Requirement: Creating an API key

The system SHALL let a signed-in user with a **proven email address** create a named API key,
and SHALL refuse the request when the account's address was never proven. The server SHALL
generate an opaque, high-entropy token, return the plaintext token **exactly once** in the
creation response, and persist only a SHA-256 hash of it plus a short non-secret display prefix
— never the plaintext. Creation SHALL accept an optional expiry; absent an expiry the key SHALL
never expire.

The verified-address condition SHALL be enforced in the statement that inserts the key, not in
a check a call site could bypass. Registration issues a session before the address is proven,
so without this gate someone who registered another person's address could take away a
never-expiring, full-scope credential and keep it past the seizure that hands the account to
its proven owner.

#### Scenario: Create returns the secret once

- **WHEN** a signed-in user with a verified address sends `POST /api/v1/me/api-keys` with a `name`
- **THEN** the system responds `201` with `{"data": {id, name, token_prefix,
  created_at, expires_at, token}}` where `token` is the full plaintext key
- **AND** the stored row holds only the token's SHA-256 hash and `token_prefix`,
  not the plaintext

#### Scenario: An unverified address cannot mint a key

- **WHEN** a signed-in user whose email is not verified sends `POST /api/v1/me/api-keys`
- **THEN** the system responds `403` and persists no key

#### Scenario: The secret is never returned again

- **WHEN** the key is later listed or otherwise read
- **THEN** no endpoint returns the plaintext token or its hash again

#### Scenario: Optional expiry is honored

- **WHEN** a user creates a key with an `expires_at` in the future
- **THEN** the key authenticates until that moment and `expires_at` is reflected
  in the key's metadata
- **AND** a key created without an expiry has a null `expires_at` and does not
  expire
