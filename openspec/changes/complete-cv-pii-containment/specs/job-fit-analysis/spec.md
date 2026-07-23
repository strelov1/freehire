## ADDED Requirements

### Requirement: Fit is scored from the de-identified structured résumé

The fit chain SHALL score the candidate from the **structured résumé** with its contact fields
(`full_name`, `email`, `phone`, `links`) excluded, and SHALL NOT send the raw CV text to the
model. The structured résumé is produced once at upload (already de-identified), so the fit
analysis carries no direct identifier to the provider by construction — no per-analysis masking
is performed.

#### Scenario: Provider never sees CV PII

- **WHEN** a fit analysis runs for a user whose CV contains name/email/phone/links
- **THEN** the text sent to the model is the structured résumé without contact fields, and the raw CV is not sent

#### Scenario: No structured résumé, no analysis

- **WHEN** a user has a stored CV but no current structured résumé (extraction absent or stale)
- **THEN** the fit analysis does not run and the endpoint responds `200` with no analysis

## MODIFIED Requirements

### Requirement: Best-effort degradation

The feature SHALL degrade gracefully: when the LLM is unconfigured or the call fails, the endpoint
MUST NOT error the request and MUST leave the deterministic profile-match unaffected. When the
caller has no stored CV, the response MUST indicate `has_cv: false` and prompt an upload instead of
running the LLM. When the caller has a CV but no current structured résumé, the chain MUST degrade
to no analysis (the structured résumé is the fit input, and it is what carries the de-identified
signal).

#### Scenario: LLM unconfigured

- **WHEN** a user POSTs the fit endpoint while the LLM is not configured
- **THEN** the system responds `200` with no analysis and does not persist a cache row

#### Scenario: Caller has no stored CV

- **WHEN** a user without a stored CV requests the fit
- **THEN** the system responds `200` with `has_cv: false` and no analysis, and does not invoke the LLM

#### Scenario: No structured résumé degrades to no analysis

- **WHEN** a user POSTs the fit endpoint with a CV but no current structured résumé
- **THEN** the system responds `200` with no analysis and does not persist a cache row

## REMOVED Requirements

### Requirement: CV and structured résumé are PII-masked in the prompt-chain

**Reason**: The raw CV is no longer sent to the model at all — the fit is scored from the
de-identified structured résumé — so per-analysis masking and restore are obsolete.
**Migration**: See "Fit is scored from the de-identified structured résumé". De-identification
now happens once at résumé extraction (`cv-pii-masking` / `resume-structured-profile`); the fit
chain consumes the already-clean structured résumé and performs no masking of its own.
