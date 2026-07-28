## MODIFIED Requirements

### Requirement: Structured résumé is extracted best-effort on upload

The system SHALL, on every résumé upload (both the `PUT /api/v1/me/resume` storage path and the `POST /api/v1/me/resume/extract` path), derive a typed **structured résumé** from the uploaded CV text, persist it per user, and **import its work-experience section into the user's experience bank**. The **contact fields** (`full_name`, `email`, `phone`, `links`) SHALL be filled from deterministic PII detection over the CV, NOT from the LLM; only the **redacted** CV text SHALL be sent to the LLM, which extracts the semantic fields (summary, experience, education, skills, …). The extraction SHALL run **off the upload response path** (in the background, like the existing CV embedding) and SHALL be **best-effort**: when the LLM is not configured, when the PII detector is unconfigured or unavailable, or when extraction fails, the upload, the CV embedding, the experience bank, and the deterministic extractors (`cv-autofill`, skilltag) MUST be unaffected and no structured résumé is persisted for that attempt. The CV bytes and text MUST NOT be logged.

#### Scenario: Upload derives, stores, and imports the structured résumé

- **WHEN** a signed-in user uploads a résumé and both the LLM and the PII detector are configured
- **THEN** the system fills the contact fields from PII detection, sends only the redacted CV to the LLM for the semantic fields, persists the merged structured résumé stamped with the producing model and the résumé's upload time, and imports its work experience into the user's experience bank

#### Scenario: LLM unconfigured leaves upload unaffected

- **WHEN** a résumé is uploaded and the LLM integration is not configured
- **THEN** the résumé is stored and embedded exactly as before, no structured résumé is persisted and nothing is imported into the bank, with no error surfaced to the upload

#### Scenario: PII detector unavailable is fail-closed

- **WHEN** a résumé is uploaded while the PII detector is unconfigured or failing
- **THEN** no CV text is sent to the LLM, no structured résumé is persisted, and nothing is imported into the bank, leaving the upload and embedding unaffected

#### Scenario: Extraction failure is swallowed

- **WHEN** the LLM call fails or returns unparseable output during extraction
- **THEN** the failure is logged without the CV contents, the upload response is unaffected, and both the previously stored structured résumé and the existing experience bank are left in place

### Requirement: The structured résumé is read-only and tied to the current résumé

The stored structured résumé SHALL be read-only and SHALL always describe the résumé currently stored for the user, **for the sections it still owns** — contacts, summary, education, languages, links, and the total-years estimate. A re-upload re-derives it, and it is served only when its stamp matches the current résumé's upload time; a structured résumé whose stamp does not match MUST be treated as absent rather than served. Deleting the résumé SHALL clear the stored structured résumé.

Work experience is NOT governed by this rule. Experience extracted from a CV is imported into the experience bank, which is durable, additive, and independent of the current résumé's stamp: a pending extraction, a superseded structure, or a deleted résumé MUST NOT hide or remove banked experience.

#### Scenario: Re-upload re-derives the structure

- **WHEN** a user who already has a structured résumé uploads a new CV
- **THEN** the structured résumé is re-derived from the new CV in the background and, once persisted, replaces the previous one

#### Scenario: A structure from a superseded résumé is not served

- **WHEN** a newer résumé has been uploaded but its structured extraction has not yet completed
- **THEN** the read surface reports no structured résumé rather than the structure derived from the superseded CV

#### Scenario: Banked experience survives a pending extraction

- **WHEN** a newer résumé has been uploaded and its extraction has not yet completed
- **THEN** the user's experience bank still serves their employments and atoms, and the fit analysis still has a candidate context

#### Scenario: Deleting the résumé clears the structure but not the bank

- **WHEN** a signed-in user deletes their stored résumé
- **THEN** the stored structured résumé is cleared along with the résumé pointer, and the experience bank is left intact

### Requirement: The structured résumé is served on the résumé read surface

The system SHALL expose the current structured résumé on the authenticated résumé status read (`GET /api/v1/me/resume`), so the profile UI can render the parsed sections, **with its work-experience section served from the experience bank rather than from the stored structure**. The non-experience fields SHALL be null when the user has no résumé, the LLM is unconfigured, extraction has not completed, or the stored structure is stale relative to the current résumé; the experience section SHALL be served whenever the bank is non-empty, independently of that staleness. The wire shape SHALL be generated to TypeScript via `cmd/gen-contracts`.

#### Scenario: The read surface serves banked experience beside the parsed sections

- **WHEN** an authenticated user with a current structured résumé and a populated bank reads their résumé status
- **THEN** the response carries the parsed contacts, education and languages from the structure, and the work experience from the bank

#### Scenario: Experience is served while the rest is stale

- **WHEN** an authenticated user's structured résumé is stale but their bank is populated
- **THEN** the response carries no parsed contacts or education, and still carries their banked work experience
