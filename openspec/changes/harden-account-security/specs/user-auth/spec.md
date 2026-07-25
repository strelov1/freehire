## MODIFIED Requirements

### Requirement: User registration

The system SHALL allow a new user to register with an email and password,
creating exactly one account per email, starting a session, and mailing a
verification code for the address on success.

- Email MUST be unique (case-insensitive); the stored form is lowercased.
- Password MUST be at least 8 characters; it is stored only as a bcrypt hash,
  never in plaintext and never returned in any response.
- The new account MUST be recorded as email-unverified, and a verification code
  MUST be mailed to the address (see the `email-verification` capability).
- On success the system returns the created user (id, email, created_at,
  verified state) and sets the httpOnly session cookie carrying a signed JWT.
- A mail-delivery failure MUST NOT fail the registration.

#### Scenario: Successful registration

- **WHEN** a client POSTs a unique, well-formed email and an 8+ character password to `/api/v1/auth/register`
- **THEN** the system creates the user as unverified, stores a bcrypt hash of the password, mails a verification code, and responds `201` with the user (no password hash) and a `Set-Cookie` carrying the session token

#### Scenario: Duplicate email

- **WHEN** a client registers with an email that already exists (in any letter case)
- **THEN** the system responds `409` and creates no new account

#### Scenario: Invalid input

- **WHEN** a client submits a malformed email or a password shorter than 8 characters
- **THEN** the system responds `400` and creates no account

#### Scenario: Mail delivery failure does not fail registration

- **WHEN** the verification mail cannot be sent
- **THEN** the account is still created, the session cookie is still set, and the client can request the code again later

### Requirement: Stateless cookie session

The system SHALL issue stateless JWTs (HS256) on register and login, delivered
in an httpOnly cookie, and SHALL validate that cookie — including a revocation
check against the account's current token version — on protected requests.

- The token SHALL encode the user id as its subject, a token-version claim, and
  an expiry.
- The cookie SHALL be `HttpOnly` and `SameSite=Lax`, with `Secure` configurable
  (set in HTTPS deployments) and a max-age matching the token expiry.
- A protected handler MUST be able to resolve the authenticated user's id from
  the validated cookie.
- A token whose version claim does not equal the account's stored token version
  SHALL be rejected, as SHALL a token carrying no version claim.

#### Scenario: Valid cookie grants access

- **WHEN** a client calls a protected endpoint with a valid, unexpired session cookie whose token version matches the account
- **THEN** the system resolves the user from the cookie and serves the request

#### Scenario: Missing cookie

- **WHEN** a client calls a protected endpoint with no session cookie
- **THEN** the system responds `401` and does not serve the protected resource

#### Scenario: Expired or invalid signature

- **WHEN** a client calls a protected endpoint with an expired cookie or one whose signature does not verify against the server secret
- **THEN** the system responds `401`

#### Scenario: Revoked token version

- **WHEN** a client presents a correctly signed, unexpired token whose version claim is lower than the account's stored token version
- **THEN** the system responds `401`

#### Scenario: Token minted before versioning

- **WHEN** a client presents a correctly signed token that carries no version claim
- **THEN** the system responds `401`

### Requirement: External identity account resolution

The system SHALL key external identities by `(provider, provider_user_id)` in
a `user_identities` table referencing `users`, resolving each OAuth sign-in to
exactly one account, and SHALL never merge a provider identity into an account
whose email ownership was never proven without first neutralising that account's
password credential.

- A sign-in matching an existing identity SHALL resolve to its linked user.
- A first-time identity whose **verified** provider email matches an existing
  **verified** account (case-insensitive) SHALL be linked to that account,
  leaving its password intact.
- A first-time identity whose **verified** provider email matches an existing
  **unverified**, password-backed account SHALL be linked to that account, and
  the system SHALL clear that account's `password_hash`, mark it verified, and
  revoke its existing sessions — the address's proven owner takes the account
  and any squatter's password stops working.
- A first-time identity with no matching account SHALL create a new
  passwordless user (`password_hash` NULL), marked verified, plus the identity,
  in one transaction.
- An identity whose provider email is unverified or absent SHALL NOT be linked
  by email and SHALL NOT create an account keyed to that email; the sign-in
  fails.

#### Scenario: Returning OAuth user

- **WHEN** a user signs in via a provider identity that already exists
- **THEN** the system starts a session for the linked user without touching `users` or creating rows

#### Scenario: First OAuth sign-in linking an existing verified account

- **WHEN** a user signs in via a new provider identity whose verified email matches an existing verified account
- **THEN** the system adds the identity linked to that account and starts a session for it, leaving the password intact

#### Scenario: First OAuth sign-in seizing an unverified account

- **WHEN** a user signs in via a new provider identity whose verified email matches an existing unverified account that has a password
- **THEN** the system links the identity, clears the stored password hash, marks the account verified, bumps its token version, and starts a session — so the previous password and any session minted with it no longer authenticate

#### Scenario: First OAuth sign-in creating a new account

- **WHEN** a user signs in via a new provider identity whose verified email matches no account
- **THEN** the system creates a passwordless, verified user and the identity in one transaction and starts a session

#### Scenario: Unverified provider email never links

- **WHEN** a provider returns an identity whose email is unverified or missing
- **THEN** the system links nothing, creates nothing, and redirects to the SPA with `auth_error`

### Requirement: OAuth provider sign-in

The system SHALL support sign-in via external OAuth providers (Google, GitHub,
LinkedIn) using the server-side authorization-code flow, issuing the same
httpOnly cookie session as password auth, and SHALL derive the flow's redirect
origin only from an explicitly configured host.

- Each provider SHALL be enabled only when its client id and client secret are
  configured; routes for unknown or disabled providers respond `404`.
- The flow SHALL be protected against CSRF by a random `state` value carried in
  a short-lived httpOnly cookie and verified on callback.
- The request's `Host` SHALL be honoured as the redirect origin only when it
  matches a configured served host exactly; any other `Host` SHALL fall back to
  the canonical frontend origin. Suffix matching against a cookie domain SHALL
  NOT be used, so a hijacked subdomain cannot steer the flow.
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

#### Scenario: Unlisted host does not steer the redirect

- **WHEN** the request arrives with a `Host` that is not in the configured served-host list (for example a subdomain that is not served by the app)
- **THEN** the system builds the provider redirect and the post-login redirect from the canonical frontend origin instead of that `Host`

## ADDED Requirements

### Requirement: Global session revocation

The system SHALL let a signed-in user invalidate every session issued for their account,
including the caller's own, by bumping a per-account token version that every authenticated
request is checked against.

- `POST /api/v1/auth/logout-all` SHALL be reachable with a session cookie only.
- The endpoint SHALL bump the account's token version and clear the caller's cookie.
- Password reset, password change, and an OAuth seizure of an unverified account SHALL bump
  the same counter.

#### Scenario: Sign out everywhere

- **WHEN** a signed-in user POSTs to `/api/v1/auth/logout-all`
- **THEN** the system bumps the account's token version, clears the caller's session cookie, and every other session for that account fails its next authenticated request with `401`

#### Scenario: API keys survive a session revocation

- **WHEN** the account's token version is bumped
- **THEN** the account's live API keys continue to authenticate, since they are revoked individually

#### Scenario: Revocation requires a session

- **WHEN** the request presents no session cookie, or authenticates with an API key
- **THEN** the system responds `401` and bumps nothing
