## MODIFIED Requirements

### Requirement: Structured résumé is extracted best-effort on upload

The system SHALL, on every résumé upload (both the `PUT /api/v1/me/resume` storage path and the `POST /api/v1/me/resume/extract` path), derive a typed **structured résumé** from the uploaded CV text and persist it per user. The **contact fields** (`full_name`, `email`, `phone`, `links`, and the candidate's own `location`) SHALL be filled from deterministic PII detection over the CV, NOT from the LLM; only the **redacted** CV text SHALL be sent to the LLM, which extracts the semantic fields (summary, experience, education, skills, …). The extraction SHALL run **off the upload response path** (in the background, like the existing CV embedding) and SHALL be **best-effort**: when the LLM is not configured, when the PII detector is unconfigured or unavailable, or when extraction fails, the upload, the CV embedding, and the deterministic extractors (`cv-autofill`, skilltag) MUST be unaffected and no structured résumé is persisted for that attempt. The CV bytes and text MUST NOT be logged.

The candidate's residence is an address span the detector masks before the LLM sees the CV. Leaving `location` to the model therefore asks it to recover a value that was deliberately removed; the system MUST restore it from detection the same way it restores name, email, phone and links.

#### Scenario: Upload derives and stores the structured résumé

- **WHEN** a signed-in user uploads a résumé and both the LLM and the PII detector are configured
- **THEN** the system fills the contact fields (including location) from PII detection, sends only the redacted CV to the LLM for the semantic fields, and persists the merged structured résumé stamped with the producing model and the résumé's upload time

#### Scenario: Candidate location survives redaction

- **WHEN** the CV states a candidate residence that the detector tags as an address span
- **THEN** the persisted structured résumé's `location` carries that residence, even though the LLM only saw a redacted placeholder for it

#### Scenario: LLM unconfigured leaves upload unaffected

- **WHEN** a résumé is uploaded and the LLM integration is not configured
- **THEN** the résumé is stored and embedded exactly as before and no structured résumé is persisted, with no error surfaced to the upload

#### Scenario: PII detector unavailable is fail-closed

- **WHEN** a résumé is uploaded while the PII detector is unconfigured or failing
- **THEN** no CV text is sent to the LLM and no structured résumé is persisted, leaving the upload and embedding unaffected

#### Scenario: Extraction failure is swallowed

- **WHEN** the LLM call fails or returns unparseable output during extraction
- **THEN** the failure is logged without the CV contents and the upload response is unaffected, leaving any previously stored structured résumé in place
