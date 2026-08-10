## MODIFIED Requirements

### Requirement: OAuth provider sign-in

The system SHALL support sign-in via external OAuth providers (Google, GitHub,
LinkedIn, Apple) using the server-side authorization-code flow, issuing the same
httpOnly cookie session as password auth, and SHALL derive the flow's redirect
origin only from an explicitly configured host.

- Each provider SHALL be enabled only when its full credential set is configured:
  a client id and client secret for Google/GitHub/LinkedIn, or a client id, Team
  ID, Key ID, and private key for Apple (which authenticates with a
  freehire-signed JWT instead of a static client secret). Routes for unknown or
  disabled providers respond `404`.
- The flow SHALL be protected against CSRF by a random `state` value carried in
  a short-lived httpOnly cookie and verified on callback.
- The request's `Host` SHALL be honoured as the redirect origin only when it
  matches a configured served host exactly; any other `Host` SHALL fall back to
  the canonical frontend origin. Suffix matching against a cookie domain SHALL
  NOT be used, so a hijacked subdomain cannot steer the flow.
- On success the callback SHALL set the session cookie and redirect the
  browser to the SPA; on any failure it SHALL redirect to the SPA with an
  `auth_error` query parameter, never rendering a JSON error to the browser.
- A provider whose callback requires `response_mode=form_post` (Apple, when the
  `email` scope is requested) SHALL be accepted as a `POST` with a form-encoded
  body carrying `state` and `code`, in addition to the `GET` query-parameter form
  every other provider uses.
- An identity provider that returns claims only inside a signed token (Apple's
  `id_token`, with no separate userinfo endpoint) SHALL have that token's
  signature verified against the provider's published JWKS before any claim is
  trusted for account resolution.

#### Scenario: Start redirects to the provider

- **WHEN** a client requests `GET /api/v1/auth/oauth/:provider/start` for an enabled provider
- **THEN** the system sets a state cookie and responds `302` to the provider's consent URL carrying the same state

#### Scenario: Successful callback signs the user in

- **WHEN** the provider redirects back to the callback with a valid code and a state matching the state cookie
- **THEN** the system resolves the account, sets the httpOnly session cookie, and redirects to the SPA, where `GET /me` returns the user

#### Scenario: Apple callback arrives as a POST

- **WHEN** Apple redirects back to the callback as a `POST` with a form-encoded body carrying a valid `code` and a `state` matching the state cookie
- **THEN** the system resolves the account and signs the user in exactly as it would for a `GET` callback from any other provider

#### Scenario: Apple id_token with an invalid signature is rejected

- **WHEN** the Apple token exchange returns an `id_token` whose signature does not verify against Apple's published JWKS
- **THEN** the system sets no session cookie and redirects to the SPA with `auth_error`

#### Scenario: State mismatch is rejected

- **WHEN** the callback's `state` does not match the state cookie (or the cookie is absent)
- **THEN** the system sets no session cookie and redirects to the SPA with `auth_error`

#### Scenario: Unknown or disabled provider

- **WHEN** a client requests the start or callback route for a provider that is not configured
- **THEN** the system responds `404`

#### Scenario: Apple disabled when its credential set is incomplete

- **WHEN** the Apple client id is configured but the Team ID, Key ID, or private key is missing
- **THEN** the system treats Apple as disabled — it is absent from the provider list and its routes respond `404`

#### Scenario: Unlisted host does not steer the redirect

- **WHEN** the request arrives with a `Host` that is not in the configured served-host list (for example a subdomain that is not served by the app)
- **THEN** the system builds the provider redirect and the post-login redirect from the canonical frontend origin instead of that `Host`
