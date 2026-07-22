## ADDED Requirements

### Requirement: CV and structured résumé are PII-masked in the prompt-chain

The fit chain SHALL mask PII in the CV text and the structured-résumé JSON on the way into
every stage prompt (Extract & Match, Recruiter verdict, Adversarial audit), so no direct
identifier reaches the model provider. It SHALL restore the original values only in the
user-facing output — the streamed sections and the returned/cached analysis — and MUST NOT
restore any data that is threaded back into a later stage's prompt.

#### Scenario: Provider never sees CV PII

- **WHEN** a fit analysis runs for a user with a CV containing name/email/phone/links
- **THEN** the text sent to the LLM in every stage carries placeholders, not the real identifiers

#### Scenario: Output is restored for the user

- **WHEN** the model echoes a masked value in an evidence or comment field
- **THEN** the emitted and returned/cached analysis show the real value, restored from the redactor

#### Scenario: No re-leak into later stages

- **WHEN** Stage 1 requirements are fed into the Stage 2 and Stage 3 prompts
- **THEN** the threaded requirement text remains masked (restore applies only to the outbound copy)

## MODIFIED Requirements

### Requirement: Best-effort degradation

The feature SHALL degrade gracefully: when the LLM is unconfigured or the call fails, the endpoint
MUST NOT error the request and MUST leave the deterministic profile-match unaffected. When the
caller has no stored CV, the response MUST indicate `has_cv: false` and prompt an upload instead of
running the LLM. When the PII detector is unconfigured or unavailable, the chain SHALL be
fail-closed: it MUST NOT send the CV to the LLM and MUST degrade to no analysis, exactly as when
the LLM is unconfigured.

#### Scenario: LLM unconfigured

- **WHEN** a user POSTs the fit endpoint while the LLM is not configured
- **THEN** the system responds `200` with no analysis and does not persist a cache row

#### Scenario: Caller has no stored CV

- **WHEN** a user without a stored CV requests the fit
- **THEN** the system responds `200` with `has_cv: false` and no analysis, and does not invoke the LLM

#### Scenario: PII detector unavailable is fail-closed

- **WHEN** a user POSTs the fit endpoint while the PII detector is unconfigured or failing
- **THEN** the system responds `200` with no analysis, does not send the CV to the LLM, and does not persist a cache row
