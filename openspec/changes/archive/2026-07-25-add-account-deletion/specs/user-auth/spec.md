## MODIFIED Requirements

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
- The revocation check SHALL fail closed: when the account's token version cannot
  be read at all — including because the account no longer exists — the request
  SHALL be unauthenticated. A cryptographically valid token is therefore never by
  itself proof that its subject is still a member.

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

#### Scenario: Token for a deleted account

- **WHEN** a client calls a protected endpoint with an unexpired, correctly signed cookie whose subject is an account that has been deleted
- **THEN** the system responds `401`, rather than admitting the request and failing later on a missing user
