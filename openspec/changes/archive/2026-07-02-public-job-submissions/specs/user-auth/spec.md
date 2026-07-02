## MODIFIED Requirements

### Requirement: Current user endpoint

The system SHALL expose `GET /api/v1/auth/me` that returns the authenticated
user's profile. It is reachable with a valid session cookie OR an API key, so a
non-browser client (e.g. the CLI) can resolve its own identity; it is a read of
the caller's own user, not key management (which stays cookie-only).

The returned user profile SHALL include the user's `role` so a client can decide
whether to surface moderator-only UI. The `role` is an affordance only — authorization
is always enforced server-side by `RequireRole`, which loads the role from the database
per request and never trusts a client-supplied value. The password hash MUST never be
included.

#### Scenario: Authenticated by session cookie

- **WHEN** an authenticated client calls `GET /api/v1/auth/me` with a valid session cookie
- **THEN** the system responds `200` with the user (id, email, role, created_at) and never includes the password hash

#### Scenario: Authenticated by API key

- **WHEN** a client calls `GET /api/v1/auth/me` with a valid `Authorization: Bearer <key>` and no cookie
- **THEN** the system responds `200` with the key owner's user (id, email, role, created_at)

#### Scenario: Unauthenticated request

- **WHEN** a client calls `GET /api/v1/auth/me` with neither a valid session cookie nor a valid API key
- **THEN** the system responds `401`
