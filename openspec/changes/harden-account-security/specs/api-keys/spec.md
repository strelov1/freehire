## ADDED Requirements

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
