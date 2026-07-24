# extension-auth Specification

## Purpose
TBD - created by archiving change add-extension-connect-auth. Update Purpose after archive.
## Requirements
### Requirement: Extension connect is session-only

The connect endpoint SHALL authenticate by session cookie only, exactly like
other key-management endpoints. A request without a valid session cookie SHALL be
rejected and no API key SHALL be minted. Presenting an `Authorization: Bearer`
API key SHALL NOT authorize the connect flow — a leaked key MUST NOT be able to
mint further keys.

#### Scenario: Anonymous request is rejected
- **WHEN** the connect endpoint is called without a valid session cookie
- **THEN** the server responds `401` and mints no key

#### Scenario: An API key cannot drive connect
- **WHEN** the connect endpoint is called with only `Authorization: Bearer <key>` and no session cookie
- **THEN** the server responds `401` and mints no key

### Requirement: Redirect target is validated against an allowlist

Before doing anything else, the server SHALL validate the supplied `redirect_uri`.
It SHALL accept only an `https://<extension-id>.chromiumapp.org/` URL whose
`<extension-id>` is on a server-configured allowlist. Any other scheme, host, or
a non-allowlisted extension id SHALL be rejected with `400` and SHALL mint no key.

#### Scenario: Allowlisted extension redirect is accepted
- **WHEN** connect is called with `redirect_uri=https://<allowlisted-id>.chromiumapp.org/` and a valid session
- **THEN** the flow proceeds to the consent step

#### Scenario: Non-allowlisted or malformed redirect is rejected
- **WHEN** connect is called with a `redirect_uri` that is not an allowlisted `chromiumapp.org` extension URL (wrong host, wrong scheme, or unknown extension id)
- **THEN** the server responds `400` and mints no key

### Requirement: Consent mints a named key and returns it via the fragment

The flow SHALL require an explicit user consent action before minting. On consent,
the server SHALL mint a named API key using the existing API-key facility (stored
hashed, plaintext generated once) and SHALL `302`-redirect to the validated
`redirect_uri` with the plaintext token and the caller's opaque `state` placed in
the URL **fragment** (not the query string or path), so the token is not carried
in request lines, `Referer` headers, or server logs.

#### Scenario: Approval returns the token in the fragment
- **WHEN** a signed-in user approves consent for an allowlisted `redirect_uri` carrying `state=abc`
- **THEN** the server mints a named API key
- **AND** it `302`-redirects to the `redirect_uri` with the plaintext token and `state=abc` in the URL fragment

#### Scenario: The minted key is attributed to the extension
- **WHEN** a key is minted through the connect flow
- **THEN** it is created with a recognizable name and belongs to the consenting user

### Requirement: Declining consent mints nothing

The user SHALL be able to decline. On decline, the server SHALL mint no key and
SHALL return control to the extension with an error indication rather than a token,
so the extension's auth flow resolves deterministically instead of hanging.

#### Scenario: Decline yields an error, not a token
- **WHEN** a signed-in user declines consent
- **THEN** no key is minted
- **AND** the redirect (or response) carries an error indication and no token

### Requirement: The minted token is an ordinary revocable API key

A token minted by the connect flow SHALL be a normal API key: it SHALL authenticate
per-user endpoints via `Authorization: Bearer <token>`, SHALL appear in the user's
key listing, and SHALL be revocable. After revocation the token MUST no longer
authenticate any request.

#### Scenario: The token authenticates as the user
- **WHEN** the extension calls a per-user endpoint with `Authorization: Bearer <minted-token>`
- **THEN** the request is authenticated as the consenting user

#### Scenario: The token is listed and revocable
- **WHEN** the user lists their API keys after connecting
- **THEN** the extension key appears in the list
- **AND** revoking it causes the minted token to stop authenticating

