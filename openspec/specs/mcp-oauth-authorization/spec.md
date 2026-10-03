# mcp-oauth-authorization Specification

## Purpose
TBD - created by archiving change mcp-oauth-signin. Update Purpose after archive.
## Requirements
### Requirement: Dynamic Client Registration
The system SHALL accept public, unauthenticated client registration per RFC 7591 at `POST /api/v1/oauth/register`, requiring a `client_name` and at least one `redirect_uris` entry, and responding with a server-generated `client_id`, `token_endpoint_auth_method: "none"`, `grant_types: ["authorization_code"]`, and `response_types: ["code"]`.

#### Scenario: A new MCP client registers successfully
- **WHEN** a client POSTs `{"client_name": "Claude Desktop", "redirect_uris": ["http://127.0.0.1:51739/callback"]}` to `/api/v1/oauth/register`
- **THEN** the response is `201 Created` with a non-empty `client_id`, the echoed `redirect_uris`, and `token_endpoint_auth_method: "none"`

#### Scenario: Registration without a redirect URI is rejected
- **WHEN** a client POSTs a body with `client_name` but an empty or missing `redirect_uris`
- **THEN** the response is `400 Bad Request` and no client row is created

### Requirement: Authorization Server and Protected Resource Metadata
The system SHALL serve `GET /.well-known/oauth-authorization-server` (RFC 8414) naming the registration, authorization, and token endpoints and declaring `code_challenge_methods_supported: ["S256"]`, and SHALL serve `GET /.well-known/oauth-protected-resource` (RFC 9728) naming the signed-in MCP server as the protected resource and this deployment as its authorization server.

#### Scenario: A client discovers the authorization server
- **WHEN** a client GETs `/.well-known/oauth-authorization-server`
- **THEN** the response names `authorization_endpoint`, `token_endpoint`, and `registration_endpoint`, and `code_challenge_methods_supported` is exactly `["S256"]`

#### Scenario: A client discovers which authorization server protects the MCP resource
- **WHEN** a client GETs `/.well-known/oauth-protected-resource`
- **THEN** the response's `resource` names the `/api/v1/mcp/account` endpoint and `authorization_servers` names this deployment's origin

### Requirement: Consent Authorization Flow
The system SHALL show a consent screen on `GET /api/v1/oauth/authorize` for a signed-in visitor presenting a valid `client_id`, a `redirect_uri` registered to that client, `response_type=code`, and `code_challenge_method=S256`; SHALL redirect a sessionless visitor to sign in and return to the same authorize request; and SHALL act on the decision only via `POST /api/v1/oauth/authorize`, issuing a single-use authorization code on approval and an `access_denied` error on refusal, in both cases as a 302 redirect to the client's `redirect_uri` carrying the original `state`.

#### Scenario: A signed-in visitor sees the consent screen
- **WHEN** a signed-in user's browser GETs `/api/v1/oauth/authorize` with a valid registered client, redirect_uri, `response_type=code`, and a PKCE `code_challenge`/`code_challenge_method=S256`
- **THEN** the response is `200 OK` HTML showing an Allow/Deny form naming the client

#### Scenario: A sessionless visitor is sent to sign in first
- **WHEN** a visitor with no session cookie GETs `/api/v1/oauth/authorize` with a valid request
- **THEN** the response is a redirect to the sign-in page carrying a `returnTo` that re-enters this same authorize request

#### Scenario: An unregistered redirect_uri is refused before any consent is shown
- **WHEN** `GET /api/v1/oauth/authorize` is called with a `redirect_uri` not in the named client's registered list
- **THEN** the response is `400 Bad Request` and no authorization code is issued

#### Scenario: Approval issues a code and redirects with state preserved
- **WHEN** a signed-in user POSTs `decision=allow` to `/api/v1/oauth/authorize` with a valid request
- **THEN** the response is a `302` redirect to the client's `redirect_uri` with a `code` and the original `state` in the query string, and a new row exists recording that code's hash, the requesting client, the user, the redirect_uri, and the PKCE `code_challenge`

#### Scenario: Refusal redirects with an error and issues no code
- **WHEN** a signed-in user POSTs `decision=deny` (or any value other than `allow`)
- **THEN** the response is a `302` redirect to the client's `redirect_uri` with `error=access_denied` and the original `state`, and no authorization code is created

### Requirement: Loopback Redirect URI Matching
The system SHALL match a candidate `redirect_uri` against a client's registered URIs by exact string equality, except that a loopback redirect (host `127.0.0.1`, `::1`, or `localhost`) SHALL match a registered loopback URI with the same scheme and path regardless of port.

#### Scenario: A loopback redirect on a different port is accepted
- **WHEN** a client registered `http://127.0.0.1:51739/callback` and an authorize request presents `redirect_uri=http://127.0.0.1:60123/callback`
- **THEN** the redirect_uri is accepted as matching

#### Scenario: A loopback redirect with a different path is rejected
- **WHEN** a client registered `http://127.0.0.1:51739/callback` and an authorize request presents `redirect_uri=http://127.0.0.1:51739/other`
- **THEN** the redirect_uri is rejected as not matching

### Requirement: PKCE Token Exchange
The system SHALL accept `POST /api/v1/oauth/token` with `grant_type=authorization_code`, requiring `code`, `client_id`, `redirect_uri`, and `code_verifier`; SHALL consume the authorization code exactly once (a second exchange of the same code fails); SHALL verify the request's `client_id` and `redirect_uri` match those recorded against the code; SHALL verify `code_verifier` against the code's stored `code_challenge` under S256; and on success SHALL respond with a bearer `access_token`, `token_type: "Bearer"`, and `expires_in` of 30 days (2,592,000 seconds), persisting a grant stamped with the user's current session-generation counter.

#### Scenario: A valid code and verifier exchange for an access token
- **WHEN** `POST /api/v1/oauth/token` is called with the code from an approved consent, the matching client_id and redirect_uri, and the correct `code_verifier`
- **THEN** the response is `200 OK` with a non-empty `access_token`, `token_type: "Bearer"`, and `expires_in: 2592000`

#### Scenario: A mismatched verifier is rejected
- **WHEN** the token exchange presents a `code_verifier` that does not hash (S256) to the code's stored `code_challenge`
- **THEN** the response is `400 Bad Request` and no access token is issued

#### Scenario: A replayed code is rejected
- **WHEN** the same authorization code is exchanged a second time, even with the correct verifier
- **THEN** the second exchange responds `400 Bad Request`

### Requirement: Grant Revocation Tied to Session Generation
The system SHALL treat a grant as invalid once its stored session-generation value no longer matches the owning user's current session-generation counter, with no separate deletion required; actions that already advance that counter (sign-out-everywhere, password reset, password change) SHALL therefore invalidate every one of that user's MCP grants.

#### Scenario: Signing out everywhere invalidates an existing grant
- **WHEN** a user has a live MCP grant and then calls sign-out-everywhere (which advances their session-generation counter)
- **THEN** a subsequent request bearing that grant's access token is rejected as unauthorized

#### Scenario: A grant issued after the bump remains valid
- **WHEN** a user signs out everywhere and then completes a new consent flow
- **THEN** the newly issued grant authenticates successfully

### Requirement: Connected Devices List and Revocation
The system SHALL expose `GET /api/v1/me/oauth-grants`, cookie-authenticated, listing the caller's own grants (client name, created-at, expires-at, last-used-at, never the token itself), and `DELETE /api/v1/me/oauth-grants/:id`, cookie-authenticated, deleting a grant only when it belongs to the caller and otherwise responding `404 Not Found` without revealing whether the id exists.

#### Scenario: A user lists their connected clients
- **WHEN** a signed-in user GETs `/api/v1/me/oauth-grants`
- **THEN** the response lists each of their own grants with its client name and timestamps, and includes no token or token hash

#### Scenario: A user revokes their own grant
- **WHEN** a signed-in user DELETEs `/api/v1/me/oauth-grants/{id}` for a grant they own
- **THEN** the response is `204 No Content` and that grant's access token no longer authenticates

#### Scenario: A user cannot revoke another user's grant
- **WHEN** a signed-in user DELETEs `/api/v1/me/oauth-grants/{id}` for a grant owned by a different user
- **THEN** the response is `404 Not Found` and the other user's grant is unaffected
