## MODIFIED Requirements

### Requirement: Fit is scored from the de-identified structured résumé

The fit chain SHALL score the candidate from a **de-identified candidate profile composed of the
experience bank and the structured résumé** — the banked employments and their publishable atoms
for the work history, the structured résumé's education, languages and summary for the rest — with
the contact fields (`full_name`, `email`, `phone`, `links`) excluded, and SHALL NOT send the raw CV
text to the model. Both sources are already de-identified at rest, so the fit analysis carries no
direct identifier to the provider by construction — no per-analysis masking is performed. A
non-publishable (`agent_inferred`) atom MUST NOT enter the candidate context.

#### Scenario: Provider never sees CV PII

- **WHEN** a fit analysis runs for a user whose CV contains name/email/phone/links
- **THEN** the text sent to the model is the composed candidate profile without contact fields, and the raw CV is not sent

#### Scenario: Experience confirmed in chat is scored

- **WHEN** a fit analysis runs for a user whose bank holds experience they confirmed in an assistant session and that appears in no uploaded CV
- **THEN** that experience is part of the candidate context the model scores against

#### Scenario: An empty bank means no analysis

- **WHEN** a user has a stored CV but an empty experience bank (import absent or failed)
- **THEN** the fit analysis does not run and the endpoint responds `200` with no analysis

#### Scenario: A stale structured résumé no longer blocks analysis

- **WHEN** a user's structured résumé is stale but their experience bank is populated
- **THEN** the fit analysis runs on the banked experience, with the education and language sections omitted

#### Scenario: Unconfirmed evidence is withheld from the model

- **WHEN** a fit analysis runs for a user whose bank contains `agent_inferred` atoms
- **THEN** those atoms are absent from the candidate context
