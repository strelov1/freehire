# api-keys Specification

## Purpose
TBD - created by archiving change add-api-keys. Update Purpose after archive.
## Requirements
### Requirement: Creating an API key

The system SHALL let a signed-in user create a named API key. The server SHALL
generate an opaque, high-entropy token, return the plaintext token **exactly
once** in the creation response, and persist only a SHA-256 hash of it plus a
short non-secret display prefix — never the plaintext. Creation SHALL accept an
optional expiry; absent an expiry the key SHALL never expire.

#### Scenario: Create returns the secret once

- **WHEN** a signed-in user sends `POST /api/v1/me/api-keys` with a `name`
- **THEN** the system responds `201` with `{"data": {id, name, token_prefix,
  created_at, expires_at, token}}` where `token` is the full plaintext key
- **AND** the stored row holds only the token's SHA-256 hash and `token_prefix`,
  not the plaintext

#### Scenario: The secret is never returned again

- **WHEN** the key is later listed or otherwise read
- **THEN** no endpoint returns the plaintext token or its hash again

#### Scenario: Optional expiry is honored

- **WHEN** a user creates a key with an `expires_at` in the future
- **THEN** the key authenticates until that moment and `expires_at` is reflected
  in the key's metadata
- **AND** a key created without an expiry has a null `expires_at` and does not
  expire

### Requirement: Listing API keys

The system SHALL let a signed-in user list their own API keys, returning metadata
only — id, name, display prefix, created time, last-used time (null until first
use), and expiry — and never the plaintext token or its hash. A user SHALL see
only their own keys.

#### Scenario: List returns metadata without secrets

- **WHEN** a signed-in user sends `GET /api/v1/me/api-keys`
- **THEN** the response is `{"data": [{id, name, token_prefix, created_at,
  last_used_at, expires_at}]}` for that user's keys, newest first
- **AND** no entry contains the plaintext token or its hash

#### Scenario: A user sees only their own keys

- **WHEN** two users each have keys and one lists their keys
- **THEN** only the calling user's keys are returned

### Requirement: Revoking an API key

The system SHALL let a signed-in user revoke one of their own keys by id. Revoking
SHALL take effect immediately: the key MUST no longer authenticate any request.
Revoking a key that does not exist or belongs to another user SHALL respond `404`
and reveal nothing about it.

#### Scenario: Revoked key stops working

- **WHEN** a user revokes a key via `DELETE /api/v1/me/api-keys/:id`
- **THEN** the system responds `204`
- **AND** a subsequent request presenting that key responds `401`

#### Scenario: Revoking a key that is not yours

- **WHEN** a user sends `DELETE /api/v1/me/api-keys/:id` for an id that does not
  exist or belongs to another user
- **THEN** the system responds `404` and changes nothing

### Requirement: Authenticating with an API key

A request that presents `Authorization: Bearer <key>` SHALL be authenticated as
the key's owning user on the per-user data endpoints (recording a view, applying,
saving/unsaving, and listing one's own jobs), granting exactly the access a
session cookie grants. An unknown, malformed, revoked, or expired key SHALL be
rejected with `401`. The session cookie SHALL continue to authenticate these
endpoints. Public read endpoints SHALL remain reachable without any key.

#### Scenario: Valid key authenticates a per-user action

- **WHEN** a client sends `POST /api/v1/jobs/:id/apply` with a valid
  `Authorization: Bearer <key>` and no cookie
- **THEN** the action is performed for the key's owning user and the system
  responds as it would for a cookie session
- **AND** the key's `last_used_at` is updated

#### Scenario: Expired or revoked key is rejected

- **WHEN** a request presents a key whose `expires_at` has passed or that has been
  revoked
- **THEN** the system responds `401` and performs no action

#### Scenario: Unknown or malformed key is rejected

- **WHEN** a request presents a Bearer value that matches no stored key hash
- **THEN** the system responds `401`

#### Scenario: Cookie still authenticates

- **WHEN** a browser request to a per-user endpoint carries a valid session cookie
  and no Authorization header
- **THEN** the request is authenticated exactly as before this change

#### Scenario: Public reads need no key

- **WHEN** a client calls `GET /api/v1/jobs`, `GET /api/v1/jobs/search`,
  `GET /api/v1/jobs/:id`, or the companies reads with no credential
- **THEN** the system responds `200` as before and performs no key lookup

### Requirement: Key management requires a session, not an API key

Key-management endpoints SHALL authenticate by session cookie only and SHALL
reject a request that presents only an API key, so a leaked key cannot create,
enumerate, or revoke keys. These endpoints are `POST`, `GET`, and `DELETE` under
`/api/v1/me/api-keys`.

#### Scenario: An API key cannot manage keys

- **WHEN** a request to any `/api/v1/me/api-keys` endpoint presents
  `Authorization: Bearer <key>` and no valid session cookie
- **THEN** the system responds `401` and the key operation is not performed

#### Scenario: A session can manage keys

- **WHEN** a signed-in user with a valid session cookie calls a
  `/api/v1/me/api-keys` endpoint
- **THEN** the operation is performed for that user

### Requirement: API keys carry a scope

The system SHALL record a scope on every API key and SHALL authorize each key-authenticated
request against that scope, so a credential minted for one narrow purpose cannot exercise the
owner's whole account.

- A key SHALL have exactly one scope: `full` (everything a key may do) or `cv` (the CV
  surface a tailoring agent needs).
- A key created by a user through `POST /api/v1/me/api-keys` SHALL be `full`; the scope
  SHALL NOT be settable by the client.
- Existing keys SHALL be `full`, so the change is invisible to current integrations.
- A key's scope SHALL be visible in the key listing, so a user can see what a credential can
  do.

#### Scenario: User-created keys are full-scope

- **WHEN** a signed-in user creates a key
- **THEN** the stored key has scope `full` and authenticates every endpoint that accepts an API key

#### Scenario: Scope is not client-settable

- **WHEN** a creation request carries a scope field
- **THEN** the field is ignored and the created key is `full`

#### Scenario: Scope is listed

- **WHEN** a user lists their keys
- **THEN** each key's metadata includes its scope

### Requirement: A narrow-scoped key is refused outside its surface

The system SHALL reject a `cv`-scoped key on any endpoint outside the CV surface, and SHALL
in particular refuse it on endpoints that expose another person's data or spend the owner's
AI credits.

- A `cv`-scoped key SHALL authenticate only the CV endpoints under `/api/v1/me/cvs` plus the
  caller's own identity read (`/api/v1/auth/me`).
- A `cv`-scoped key SHALL be refused on the employee-referral endpoints, which expose CVs
  submitted by other users.
- A `cv`-scoped key SHALL be refused on the endpoints that debit AI credits.
- A refusal SHALL be `403` and SHALL state that the credential's scope is insufficient,
  distinguishing it from an unauthenticated `401`.

#### Scenario: Narrow key reads its own CV

- **WHEN** a request carrying a `cv`-scoped key reads or patches a CV owned by the key's user
- **THEN** the request is served as it is today

#### Scenario: Narrow key cannot read a third party's referral CV

- **WHEN** a request carrying a `cv`-scoped key calls `GET /api/v1/me/referrals/incoming/:id/cv`
- **THEN** the system responds `403` and returns no document

#### Scenario: Narrow key cannot spend credits

- **WHEN** a request carrying a `cv`-scoped key POSTs a fit analysis for a vacancy
- **THEN** the system responds `403` and debits no credits

#### Scenario: Full key is unaffected

- **WHEN** the same requests are made with a `full`-scoped key
- **THEN** they are served exactly as before this change

