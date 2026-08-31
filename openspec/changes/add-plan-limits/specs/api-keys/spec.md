## MODIFIED Requirements

### Requirement: A narrow-scoped key is refused outside its surface

The system SHALL reject a `cv`-scoped key on any endpoint outside the CV surface, and SHALL
in particular refuse it on endpoints that expose another person's data or consume the owner's
plan allowance.

- A `cv`-scoped key SHALL authenticate only the CV endpoints under `/api/v1/me/cvs` plus the
  caller's own identity read (`/api/v1/auth/me`).
- A `cv`-scoped key SHALL be refused on the employee-referral endpoints, which expose CVs
  submitted by other users.
- A `cv`-scoped key SHALL be refused on the endpoints that consume a plan allowance.
- A refusal SHALL be `403` and SHALL state that the credential's scope is insufficient,
  distinguishing it from an unauthenticated `401`.

#### Scenario: Narrow key reads its own CV

- **WHEN** a request carrying a `cv`-scoped key reads or patches a CV owned by the key's user
- **THEN** the request is served as it is today

#### Scenario: Narrow key cannot read a third party's referral CV

- **WHEN** a request carrying a `cv`-scoped key calls `GET /api/v1/me/referrals/incoming/:id/cv`
- **THEN** the system responds `403` and returns no document

#### Scenario: Narrow key cannot spend an allowance

- **WHEN** a request carrying a `cv`-scoped key POSTs a fit analysis for a vacancy
- **THEN** the system responds `403` and consumes no allowance

#### Scenario: Full key is unaffected

- **WHEN** the same requests are made with a `full`-scoped key
- **THEN** they are served exactly as before this change
