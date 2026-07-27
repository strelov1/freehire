## MODIFIED Requirements

### Requirement: The tailoring agent acts as the user via a scoped, short-lived credential

The system SHALL, at tailoring bootstrap, mint a short-lived API key scoped to the CV surface
for the requesting user and return it so the agent's CLI can authenticate to the CV endpoints
as that user. Patches and reads made with that key MUST be owner-scoped to the same user, so
the agent can never read or edit another user's CV, and the key's scope MUST confine it to the
CV endpoints, so a leaked tailoring credential cannot reach the rest of the owner's account.

#### Scenario: The minted key edits only its owner's CVs

- **WHEN** the agent uses the minted key to patch a CV id that belongs to a different user
- **THEN** the request is rejected as not found / forbidden and no document is mutated

#### Scenario: The minted key is confined to the CV surface

- **WHEN** the minted key is presented to an endpoint outside the CV surface — a referral CV read, or a credit-spending analysis
- **THEN** the request is refused with `403` for insufficient scope
