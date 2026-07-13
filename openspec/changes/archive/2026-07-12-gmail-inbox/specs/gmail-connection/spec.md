## ADDED Requirements

### Requirement: Connect Gmail via incremental OAuth

The system SHALL let a signed-in user grant `gmail.readonly` through Google's
incremental authorization, layered on the existing Google OAuth client so that
ordinary sign-in never requests mail access. On success it MUST store a per-user
refresh token and mark the account connected.

#### Scenario: User connects Gmail

- **WHEN** a signed-in user starts the connect flow and grants `gmail.readonly`
- **THEN** the callback exchanges the code, stores the user's refresh token, and reports the account connected

#### Scenario: Sign-in does not request mail scope

- **WHEN** a user signs in with Google (not the connect flow)
- **THEN** only the sign-in scopes are requested, never `gmail.readonly`

### Requirement: Refresh token encrypted at rest

The system SHALL store the Gmail refresh token encrypted at rest and MUST never
return it to the client.

#### Scenario: Token stored encrypted

- **WHEN** the callback persists the refresh token
- **THEN** the stored value is ciphertext, decryptable only with the server key, and no endpoint exposes it

### Requirement: Connection status and disconnect

The system SHALL expose the caller's connection status and a disconnect that
revokes the grant and purges the stored token and synced mail.

#### Scenario: Read connection status

- **WHEN** a signed-in user requests their Gmail connection status
- **THEN** the response reports whether Gmail is connected and, if so, the connected address

#### Scenario: Disconnect purges data

- **WHEN** a connected user disconnects Gmail
- **THEN** the refresh token is revoked and removed and the user's synced ATS mail is purged

### Requirement: Testing-mode consent tolerated

The system SHALL function while the Google OAuth app is unverified (testing
mode), where only added test users can complete consent.

#### Scenario: Non-test user blocked by Google

- **WHEN** a user who is not an added test user attempts to connect
- **THEN** Google blocks consent and the app surfaces a clear "not yet available" message rather than an error page
