## MODIFIED Requirements

### Requirement: The per-account gateway credential is an (id, secret) pair

The system SHALL store both the secret a model call presents and the identifier the gateway's administrative API addresses, and SHALL write and clear them as one credential. A credential holding only a secret SHALL remain usable for inference and SHALL NOT be blocked or deleted.

#### Scenario: Minting stores both halves

- **WHEN** an account makes its first AI call and no credential is stored
- **THEN** the gateway mints one, and both the secret and the gateway's own identifier are stored in the same statement

#### Scenario: A half credential is refused

- **WHEN** the gateway answers a mint with a secret but no identifier, or an identifier but no secret
- **THEN** the mint fails and nothing is stored, because a stored half would mark the account as credentialled while being unusable in one direction

#### Scenario: A credential predating the identifier keeps working

- **WHEN** an account's stored credential carries a secret and no identifier
- **THEN** its model calls proceed on that secret, and the first refusal by the gateway clears the row so the next call mints a complete pair

### Requirement: A minted credential carries a provider policy read from a template

The system SHALL copy the provider policy of a configured template credential onto every credential it mints, and SHALL NOT carry a provider list of its own. Absent a configured template, the system SHALL behave as unconfigured rather than mint credentials the gateway will refuse.

#### Scenario: The policy is copied

- **WHEN** a credential is minted
- **THEN** the template credential is read and its provider entries — providers, weights and allowed models — are sent verbatim on the new credential

#### Scenario: A template allowing no provider fails the mint

- **WHEN** the template credential carries no provider entries
- **THEN** the mint fails rather than producing a credential that is refused on its first call

#### Scenario: No template configured disables attribution

- **WHEN** no template credential is configured
- **THEN** nothing is minted and every model call goes out on the service credential, exactly as when no administrative endpoint is configured

### Requirement: Usage is reported for the account's current credential

The system SHALL report an account's own AI usage — calls made, calls failed, tokens moved — for the credential currently stored against that account, over the calendar period credits reset on. It SHALL NOT report money. It SHALL NOT mint a credential in order to answer.

#### Scenario: An account reads its own month

- **WHEN** a signed-in account requests its usage
- **THEN** the figures cover only that account's credential, and no other account's usage is visible

#### Scenario: An account that never used AI

- **WHEN** an account with no stored credential requests its usage
- **THEN** zeroes are reported, no request is made to the gateway, and no credential is minted

#### Scenario: A credential replaced mid-period

- **WHEN** an account's credential was replaced during the period being reported
- **THEN** the figures cover the current credential only, and the retired credential's calls are absent from them

### Requirement: A model call is labelled with the feature it served

The system SHALL label every model call made for a signed-in user with the feature that call served, as a value under a single dimension, and SHALL apply the label whether or not the call could be attributed to a person.

#### Scenario: An attributed call is labelled

- **WHEN** a model call is made on an account's own credential
- **THEN** the request carries the feature dimension with a bare feature name as its value

#### Scenario: An unattributed call is still labelled

- **WHEN** attribution is unavailable and the call falls back to the service credential
- **THEN** the request still carries the feature dimension
