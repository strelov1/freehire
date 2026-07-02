# user-auth

## Purpose

Give `hire` a first authenticated surface: email/password accounts with secure
(bcrypt) password storage, and stateless JWT sessions delivered in an httpOnly,
same-origin cookie (XSS-safe, no token in JS/`localStorage`). Register, login,
logout, and a guarded `me` endpoint, plus a reusable `RequireAuth` middleware
future protected routes can adopt — without gating any existing public read
endpoint. The model is email-keyed and password-nullable so future passwordless
sign-in (OAuth/magic-link) is additive.
## Requirements
### Requirement: User registration

The system SHALL allow a new user to register with an email and password,
creating exactly one account per email and starting a session on success.

- Email MUST be unique (case-insensitive); the stored form is lowercased.
- Password MUST be at least 8 characters; it is stored only as a bcrypt hash,
  never in plaintext and never returned in any response.
- On success the system returns the created user (id, email, created_at) and
  sets the httpOnly session cookie carrying a signed JWT.

#### Scenario: Successful registration

- **WHEN** a client POSTs a unique, well-formed email and an 8+ character password to `/api/v1/auth/register`
- **THEN** the system creates the user, stores a bcrypt hash of the password, and responds `201` with the user (no password hash) and a `Set-Cookie` carrying the session token

#### Scenario: Duplicate email

- **WHEN** a client registers with an email that already exists (in any letter case)
- **THEN** the system responds `409` and creates no new account

#### Scenario: Invalid input

- **WHEN** a client submits a malformed email or a password shorter than 8 characters
- **THEN** the system responds `400` and creates no account

### Requirement: User login

The system SHALL authenticate an existing user by email and password and start a
session, without revealing whether the email or the password was the failing
factor.

#### Scenario: Successful login

- **WHEN** a client POSTs a registered email and the correct password to `/api/v1/auth/login`
- **THEN** the system responds `200` with the user and sets the httpOnly session cookie

#### Scenario: Wrong password

- **WHEN** a client submits a registered email with an incorrect password
- **THEN** the system responds `401` with a generic "invalid credentials" message and sets no cookie

#### Scenario: Unknown email

- **WHEN** a client submits an email that has no account
- **THEN** the system responds `401` with the same generic "invalid credentials" message as a wrong password

#### Scenario: Account has no password

- **WHEN** a client attempts password login for an account that has no stored password hash (e.g. one created through a future passwordless sign-in method)
- **THEN** the system responds `401` with the same generic "invalid credentials" message, never treating an absent password as a match

### Requirement: Stateless cookie session

The system SHALL issue stateless JWTs (HS256) on register and login, delivered
in an httpOnly cookie, and SHALL validate that cookie on protected requests.

- The token SHALL encode the user id as its subject and carry an expiry.
- The cookie SHALL be `HttpOnly` and `SameSite=Lax`, with `Secure` configurable
  (set in HTTPS deployments) and a max-age matching the token expiry.
- A protected handler MUST be able to resolve the authenticated user's id from
  the validated cookie.

#### Scenario: Valid cookie grants access

- **WHEN** a client calls a protected endpoint with a valid, unexpired session cookie
- **THEN** the system resolves the user from the cookie and serves the request

#### Scenario: Missing cookie

- **WHEN** a client calls a protected endpoint with no session cookie
- **THEN** the system responds `401` and does not serve the protected resource

#### Scenario: Expired or invalid signature

- **WHEN** a client calls a protected endpoint with an expired cookie or one whose signature does not verify against the server secret
- **THEN** the system responds `401`

### Requirement: Session logout

The system SHALL expose `POST /api/v1/auth/logout` that clears the session
cookie. It is public and idempotent.

#### Scenario: Logout clears the session

- **WHEN** a client calls `POST /api/v1/auth/logout`
- **THEN** the system responds with a `Set-Cookie` that expires the session cookie, so subsequent protected requests are unauthenticated

#### Scenario: Logout without a session

- **WHEN** a client calls `POST /api/v1/auth/logout` with no (or an already-expired) cookie
- **THEN** the system still responds successfully, treating it as a no-op

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

### Requirement: Web client authentication

The Svelte SPA SHALL let a user register, log in, and log out from the
application layout, persist the session across reloads, and reflect the current
auth state in the top bar.

- The session SHALL live entirely in the httpOnly cookie; the SPA SHALL hold no
  token (it cannot read the cookie) and SHALL use no `localStorage`. On boot it
  SHALL resolve the session via `GET /me`; failure SHALL leave the user signed
  out without error.
- The SPA SHALL send credentials (the cookie) with API requests; the public
  jobs/companies requests SHALL remain unauthenticated either way.
- The top bar SHALL show the signed-in user's email and a logout action when
  authenticated, and Login/Register actions when not.

#### Scenario: Sign in from the layout

- **WHEN** a signed-out user submits valid credentials in the login (or register) form opened from the top bar
- **THEN** the SPA shows the user's email with a logout action in the top bar, and the session (cookie) keeps the user signed in across a page reload

#### Scenario: Log out

- **WHEN** a signed-in user activates the logout action
- **THEN** the SPA calls the logout endpoint to clear the cookie and returns the top bar to its Login/Register state

#### Scenario: Stale cookie on boot

- **WHEN** the SPA boots and `GET /me` is rejected (expired or invalid cookie)
- **THEN** the SPA presents the signed-out state without error

### Requirement: Public endpoints remain unauthenticated

The existing read endpoints SHALL remain publicly accessible without a token, so
this change adds authentication without gating current functionality.

#### Scenario: Public read without a token

- **WHEN** a client calls `GET /api/v1/jobs`, `GET /api/v1/jobs/:id`, `GET /api/v1/companies`, or `GET /api/v1/companies/:slug` without any token
- **THEN** the system serves the request as before, unaffected by the auth layer

### Requirement: OAuth provider sign-in

The system SHALL support sign-in via external OAuth providers (Google, GitHub,
LinkedIn) using the server-side authorization-code flow, issuing the same
httpOnly cookie session as password auth.

- Each provider SHALL be enabled only when its client id and client secret are
  configured; routes for unknown or disabled providers respond `404`.
- The flow SHALL be protected against CSRF by a random `state` value carried in
  a short-lived httpOnly cookie and verified on callback.
- On success the callback SHALL set the session cookie and redirect the
  browser to the SPA; on any failure it SHALL redirect to the SPA with an
  `auth_error` query parameter, never rendering a JSON error to the browser.

#### Scenario: Start redirects to the provider

- **WHEN** a client requests `GET /api/v1/auth/oauth/:provider/start` for an enabled provider
- **THEN** the system sets a state cookie and responds `302` to the provider's consent URL carrying the same state

#### Scenario: Successful callback signs the user in

- **WHEN** the provider redirects back to the callback with a valid code and a state matching the state cookie
- **THEN** the system resolves the account, sets the httpOnly session cookie, and redirects to the SPA, where `GET /me` returns the user

#### Scenario: State mismatch is rejected

- **WHEN** the callback's `state` does not match the state cookie (or the cookie is absent)
- **THEN** the system sets no session cookie and redirects to the SPA with `auth_error`

#### Scenario: Unknown or disabled provider

- **WHEN** a client requests the start or callback route for a provider that is not configured
- **THEN** the system responds `404`

### Requirement: External identity account resolution

The system SHALL key external identities by `(provider, provider_user_id)` in
a `user_identities` table referencing `users`, resolving each OAuth sign-in to
exactly one account.

- A sign-in matching an existing identity SHALL resolve to its linked user.
- A first-time identity whose **verified** provider email matches an existing
  account (case-insensitive) SHALL be linked to that account.
- A first-time identity with no matching account SHALL create a new
  passwordless user (`password_hash` NULL) plus the identity, in one
  transaction.
- An identity whose provider email is unverified or absent SHALL NOT be linked
  by email and SHALL NOT create an account keyed to that email; the sign-in
  fails.

#### Scenario: Returning OAuth user

- **WHEN** a user signs in via a provider identity that already exists
- **THEN** the system starts a session for the linked user without touching `users` or creating rows

#### Scenario: First OAuth sign-in linking an existing password account

- **WHEN** a user signs in via a new provider identity whose verified email matches an existing account
- **THEN** the system adds the identity linked to that account and starts a session for it, leaving the password intact

#### Scenario: First OAuth sign-in creating a new account

- **WHEN** a user signs in via a new provider identity whose verified email matches no account
- **THEN** the system creates a passwordless user and the identity in one transaction and starts a session

#### Scenario: Unverified provider email never links

- **WHEN** a provider returns an identity whose email is unverified or missing
- **THEN** the system links nothing, creates nothing, and redirects to the SPA with `auth_error`

### Requirement: Enabled-provider listing

The system SHALL expose `GET /api/v1/auth/oauth/providers` returning the names
of currently enabled providers, so the web client renders only usable sign-in
buttons.

#### Scenario: Listing enabled providers

- **WHEN** a client requests the provider list while only Google and GitHub are configured
- **THEN** the system responds `200` with `{"data": ["google", "github"]}` (order not significant), omitting LinkedIn

### Requirement: Web client provider sign-in

The SPA auth dialog SHALL offer "Continue with <Provider>" actions for each
enabled provider alongside the email/password form, and SHALL surface a failed
OAuth callback (the `auth_error` redirect) as a user-visible message.

#### Scenario: Provider buttons reflect configuration

- **WHEN** a signed-out user opens the auth dialog
- **THEN** the dialog shows one sign-in action per enabled provider (and none for disabled providers) above the email/password form

#### Scenario: Provider sign-in round trip

- **WHEN** the user activates a provider action and completes consent at the provider
- **THEN** the browser returns to the SPA signed in (top bar shows the user's email) with no token ever exposed to JavaScript

#### Scenario: Failed callback is surfaced

- **WHEN** the SPA loads with `auth_error` in the URL after a failed callback
- **THEN** it shows a sign-in failure message and removes the parameter from the URL

### Requirement: Users have a role

The system SHALL store a `role` on every user, one of `user`, `moderator`, or `admin`,
defaulting to `user`. The role MUST be persisted in the database (not carried in the JWT)
so that a role change takes effect immediately on the next request. Roles are granted out
of band (direct database update); there is no self-service role-management API in this
change.

#### Scenario: New user defaults to the user role

- **WHEN** a new account is created (password or OAuth sign-in)
- **THEN** its role is `user`

### Requirement: Role-based authorization

The system SHALL provide a `RequireRole` authorization layer that runs after
authentication, reads the authenticated user id, loads the current role from the database,
and rejects the request when the role does not match the required one. Authentication
failure MUST surface as `401`; an authenticated user lacking the required role MUST surface
as `403`.

#### Scenario: Authorized role passes

- **WHEN** a request authenticated as a `moderator` hits an endpoint guarded by `RequireRole("moderator")`
- **THEN** the request proceeds to the handler

#### Scenario: Wrong role is forbidden

- **WHEN** a request authenticated as a `user` hits an endpoint guarded by `RequireRole("moderator")`
- **THEN** the system responds `403`

#### Scenario: Unauthenticated request is unauthorized

- **WHEN** a request with no valid credential hits an endpoint guarded by `RequireRole`
- **THEN** the system responds `401`

