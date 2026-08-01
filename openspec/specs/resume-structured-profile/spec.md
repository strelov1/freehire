# resume-structured-profile Specification

## Purpose
TBD - created by archiving change resume-structured-profile. Update Purpose after archive.
## Requirements
### Requirement: Structured résumé is extracted best-effort on upload

The system SHALL, on every résumé upload (both the `PUT /api/v1/me/resume` storage path and the `POST /api/v1/me/resume/extract` path), derive a typed **structured résumé** from the uploaded CV text and persist it per user. The **contact fields** (`full_name`, `email`, `phone`, `links`) SHALL be filled from deterministic PII detection over the CV, NOT from the LLM; only the **redacted** CV text SHALL be sent to the LLM, which extracts the semantic fields (summary, experience, education, skills, …). The extraction SHALL run **off the upload response path** (in the background, like the existing CV embedding) and SHALL be **best-effort**: when the LLM is not configured, when the PII detector is unconfigured or unavailable, or when extraction fails, the upload, the CV embedding, and the deterministic extractors (`cv-autofill`, skilltag) MUST be unaffected and no structured résumé is persisted for that attempt. The CV bytes and text MUST NOT be logged.

#### Scenario: Upload derives and stores the structured résumé

- **WHEN** a signed-in user uploads a résumé and both the LLM and the PII detector are configured
- **THEN** the system fills the contact fields from PII detection, sends only the redacted CV to the LLM for the semantic fields, and persists the merged structured résumé stamped with the producing model and the résumé's upload time

#### Scenario: LLM unconfigured leaves upload unaffected

- **WHEN** a résumé is uploaded and the LLM integration is not configured
- **THEN** the résumé is stored and embedded exactly as before and no structured résumé is persisted, with no error surfaced to the upload

#### Scenario: PII detector unavailable is fail-closed

- **WHEN** a résumé is uploaded while the PII detector is unconfigured or failing
- **THEN** no CV text is sent to the LLM and no structured résumé is persisted, leaving the upload and embedding unaffected

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

Persisting the structure SHALL also derive and store the candidate's geography from the structure's location line, in the same statement and under the same stamp, so that a stored geography can never describe a different CV than the structure it was derived from. The derivation is deterministic and performs no I/O; the rules governing what it emits belong to the `candidate-geography` capability. Deleting the résumé SHALL clear the derived geography along with the structure.

#### Scenario: Re-upload re-derives the structure

- **WHEN** a user who already has a structured résumé uploads a new CV
- **THEN** the structured résumé is re-derived from the new CV in the background and, once persisted, replaces the previous one

#### Scenario: A structure from a superseded résumé is not served

- **WHEN** a newer résumé has been uploaded but its structured extraction has not yet completed
- **THEN** the read surface reports no structured résumé rather than the structure derived from the superseded CV

#### Scenario: Deleting the résumé clears the structure

- **WHEN** a signed-in user deletes their stored résumé
- **THEN** the stored structured résumé is cleared along with the résumé pointer

#### Scenario: Persisting the structure persists the derived geography

- **WHEN** a structured résumé is persisted for a user
- **THEN** the geography derived from its location line is stored in the same write, under the same résumé-upload stamp

#### Scenario: A superseded extraction writes neither the structure nor the geography

- **WHEN** a background extraction completes for a résumé that has since been replaced
- **THEN** the write matches no row and neither the structure nor the derived geography is changed

#### Scenario: Deleting the résumé clears the derived geography

- **WHEN** a signed-in user deletes their stored résumé
- **THEN** the derived geography is cleared along with the structure

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

### Requirement: Certifications in the structured résumé

The structured résumé shape SHALL carry a `certifications` field: a list of the credentials the résumé claims, extracted best-effort by the résumé parser. The field MUST pass the same sanitize gate as the rest of the structured shape (bounded, capped) and MUST live in the existing `resume_structured` jsonb with no new database column. A résumé parsed before this field existed reads as absent and self-heals on the next upload, exactly like the rest of the structured shape.

#### Scenario: Résumé certifications are extracted

- **WHEN** a CV lists "AWS Certified Solutions Architect" and "PMP"
- **THEN** the structured shape's `certifications` contains entries for both

#### Scenario: Older extraction without the field degrades gracefully

- **WHEN** a structured résumé was parsed before `certifications` existed
- **THEN** `certifications` reads as absent and no error occurs; it is repopulated on the next CV upload

### Requirement: The contact-free projection is the one typed seam to a model

The system SHALL express "the part of a candidate's CV a model may see" as a single typed
projection of the structured résumé, and every consumer that sends CV-derived content to a model
SHALL receive that projection as a typed value — never as a serialized blob it filters, trims or
re-projects itself.

The projection's field set SHALL be a **whitelist**: a field added to the structured résumé is
withheld from every model until it is added to the projection too. The system MUST NOT enforce
this by removing known contact fields from a serialized structured résumé, because such a rule
discloses each newly-added field by default — the opposite of what the projection exists to do.

A consumer that has no projection available for the caller SHALL be told so as a distinct state
rather than inferring it from an empty serialization, so "this user has no structured résumé"
cannot be confused with "this value serialized to nothing".

#### Scenario: A newly added structured field reaches no model

- **WHEN** a field carrying personal data is added to the structured résumé and is not added to
  the contact-free projection
- **THEN** it appears in neither the fit chain's prompt nor the ATS review's prompt, with no
  change to either consumer

#### Scenario: A model-facing consumer cannot be handed the contact-bearing value

- **WHEN** a caller assembles the input for a model-facing CV consumer
- **THEN** the input's type is the contact-free projection, so passing the full structured
  résumé is not expressible

#### Scenario: Absence is a state, not an empty string

- **WHEN** the caller has no current structured résumé
- **THEN** the reader reports absence explicitly and the consumer skips the model call, rather
  than the consumer inferring absence from empty content

