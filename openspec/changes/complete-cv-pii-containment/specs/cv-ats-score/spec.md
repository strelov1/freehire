## MODIFIED Requirements

### Requirement: Optional LLM qualitative review, nil-safe and cached per user

When an LLM is configured, the system SHALL, on request, review the candidate's **de-identified
structured résumé** (the faithfully-copied experience highlights, summary, and skills — with the
contact fields excluded) for qualitative issues (weak vs strong action verbs, achievement-vs-
responsibility bullets, and concrete fixes) and blend a content-quality score into the overall.
It SHALL NOT send the raw CV text to the model. When no LLM is configured, the call fails, or no
structured résumé is available, the endpoint SHALL return the deterministic score only (HTTP 200,
no content-quality). The derived review SHALL be cached per user keyed to the stored CV and reused
across profiles/roles; it SHALL be invalidated when the CV is replaced or deleted. Neither the raw
CV text nor any contact identifier SHALL be persisted — only the derived review.

#### Scenario: No LLM configured degrades cleanly
- **WHEN** the server has no LLM configured and a report is requested
- **THEN** the response is 200 with the deterministic score and no content-quality

#### Scenario: Review reads the structured résumé, not the raw CV
- **WHEN** the LLM qualitative review runs for a user's stored CV
- **THEN** the text sent to the model is the structured résumé without contact fields, and the raw CV is not sent

#### Scenario: No structured résumé degrades to the deterministic score
- **WHEN** a report is requested for a CV that has no current structured résumé
- **THEN** the response is 200 with the deterministic score and no content-quality

#### Scenario: LLM review is cached and reused
- **WHEN** the LLM review has run for a user's stored CV and the report is opened again (any profile/role)
- **THEN** the cached review is served without re-calling the LLM

#### Scenario: Replacing the CV invalidates the cached review
- **WHEN** the user uploads a new CV
- **THEN** the previously cached review is cleared and not shown for the new CV

#### Scenario: Only the derived review is stored
- **WHEN** a CV is reviewed
- **THEN** the stored analysis contains the content-quality and findings but no CV text or contact identifier
