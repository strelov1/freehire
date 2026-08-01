## MODIFIED Requirements

### Requirement: Fit is scored from the de-identified structured résumé

The fit chain SHALL score the candidate from the **contact-free projection** of the structured
résumé — the typed seam defined by "The contact-free projection is the one typed seam to a
model" — and SHALL NOT send the raw CV text to the model. The chain SHALL take that projection as
a typed value rather than a serialization it re-projects, so the guarantee is carried by the
input's type rather than by each producer's discipline. The structured résumé is produced once at
upload, so the fit analysis carries no direct identifier to the provider by construction — no
per-analysis masking is performed.

#### Scenario: Provider never sees CV PII

- **WHEN** a fit analysis runs for a user whose CV contains name/email/phone/links
- **THEN** the text sent to the model is the contact-free projection of the structured résumé, and the raw CV is not sent

#### Scenario: No structured résumé, no analysis

- **WHEN** a user has a stored CV but no current structured résumé (extraction absent or stale)
- **THEN** the fit analysis does not run and the endpoint responds `200` with no analysis
