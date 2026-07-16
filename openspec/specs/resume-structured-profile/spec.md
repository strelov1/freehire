# resume-structured-profile Specification

## Purpose
TBD - created by archiving change resume-structured-profile. Update Purpose after archive.
## Requirements
### Requirement: Structured résumé is extracted best-effort on upload

The system SHALL, on every résumé upload (both the `PUT /api/v1/me/resume` storage path and the `POST /api/v1/me/resume/extract` path), derive a typed **structured résumé** from the uploaded CV text using the configured LLM, and persist it per user. The extraction SHALL run **off the upload response path** (in the background, like the existing CV embedding) and SHALL be **best-effort**: when the LLM is not configured, or extraction fails, the upload, the CV embedding, and the deterministic extractors (`cv-autofill`, skilltag) MUST be unaffected and no structured résumé is persisted for that attempt. The CV bytes and text MUST NOT be logged.

#### Scenario: Upload derives and stores the structured résumé

- **WHEN** a signed-in user uploads a résumé and the LLM is configured
- **THEN** the system derives a structured résumé from the CV text in the background and persists it for that user, stamped with the producing model and the résumé's upload time

#### Scenario: LLM unconfigured leaves upload unaffected

- **WHEN** a résumé is uploaded and the LLM integration is not configured
- **THEN** the résumé is stored and embedded exactly as before and no structured résumé is persisted, with no error surfaced to the upload

#### Scenario: Extraction failure is swallowed

- **WHEN** the LLM call fails or returns unparseable output during extraction
- **THEN** the failure is logged without the CV contents and the upload response is unaffected, leaving any previously stored structured résumé in place

### Requirement: The structured résumé is a typed, sanitized contract

The structured résumé SHALL be a typed value covering the candidate's contact basics, a professional summary, work-experience entries (title, company, location, dates, a one-line context summary, achievement highlights, and per-role technology stack), education entries, languages, links, a flat skills list, portfolio projects (name, link, highlights), and an estimated total years of experience. Before it is persisted or served, the system SHALL sanitize all model output to the contract: every string length is bounded, every array cardinality is capped, the total-years estimate is coerced to a non-negative bounded integer, and empty entries are dropped. The system MUST NOT persist or serve a value outside these bounds, so untrusted CV text cannot inject unbounded or malformed content.

#### Scenario: Out-of-bounds model output is coerced before persistence

- **WHEN** the LLM returns over-long strings, an oversized list of entries, or an implausible years value
- **THEN** the sanitized structured résumé has bounded strings, a capped number of entries, and a coerced years value, and only the sanitized value is persisted and served

#### Scenario: Fields not present in the CV are omitted, not invented

- **WHEN** the CV does not state a field (e.g. no education section)
- **THEN** that part of the structured résumé is empty rather than fabricated

#### Scenario: Rich work-history detail is captured

- **WHEN** a role in the CV lists a location, achievement bullets, and a technology stack, and the CV has a skills section and portfolio projects
- **THEN** the structured résumé captures that role's location, highlights, and stack (alongside title/company/dates), and populates the top-level skills list and projects entries — so a CV seeded from it is complete

### Requirement: The structured résumé is read-only and tied to the current résumé

The stored structured résumé SHALL be read-only — this capability provides no per-field editing. It SHALL always describe the résumé currently stored for the user: a re-upload re-derives it, and it is served only when its stamp matches the current résumé's upload time. A structured résumé whose stamp does not match the current résumé (a newer CV whose extraction has not yet landed, or a persistent extraction outage) MUST be treated as absent rather than served. Deleting the résumé SHALL clear the stored structured résumé.

#### Scenario: Re-upload re-derives the structure

- **WHEN** a user who already has a structured résumé uploads a new CV
- **THEN** the structured résumé is re-derived from the new CV in the background and, once persisted, replaces the previous one

#### Scenario: A structure from a superseded résumé is not served

- **WHEN** a newer résumé has been uploaded but its structured extraction has not yet completed
- **THEN** the read surface reports no structured résumé rather than the structure derived from the superseded CV

#### Scenario: Deleting the résumé clears the structure

- **WHEN** a signed-in user deletes their stored résumé
- **THEN** the stored structured résumé is cleared along with the résumé pointer

### Requirement: The structured résumé is served on the résumé read surface

The system SHALL expose the current structured résumé on the authenticated résumé status read (`GET /api/v1/me/resume`), so the profile UI can render the parsed sections. The field SHALL be null when the user has no résumé, the LLM is unconfigured, extraction has not completed, or the stored structure is stale relative to the current résumé. The wire shape SHALL be generated to TypeScript via `cmd/gen-contracts`.

#### Scenario: Present structured résumé is returned

- **WHEN** a signed-in user with a current structured résumé requests their résumé status
- **THEN** the response includes the sanitized structured résumé alongside the existing résumé metadata

#### Scenario: Absent structured résumé is null

- **WHEN** a signed-in user has a résumé but no current structured résumé (unconfigured LLM, not yet extracted, or stale)
- **THEN** the response reports the structured résumé as null and the rest of the résumé status is unaffected

### Requirement: The profile page renders the structured résumé read-only

The web profile SHALL render the structured résumé's sections (experience, education, contacts, languages, links, summary) read-only when one is present, and SHALL omit the structured section entirely when it is null, without error.

#### Scenario: Profile shows parsed sections

- **WHEN** a signed-in user with a current structured résumé opens their profile
- **THEN** the profile renders the parsed sections read-only

#### Scenario: Profile omits the section when absent

- **WHEN** the user has no current structured résumé
- **THEN** the profile omits the structured section and shows no error

