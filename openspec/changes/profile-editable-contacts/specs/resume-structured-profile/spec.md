## ADDED Requirements

### Requirement: Résumé meta reports parse status and supports retry

`GET /api/v1/me/resume` SHALL include a parse status for the structured extract that distinguishes at least: **current** (stamp matches upload), **pending** (résumé present, stamp missing or mismatched, no recorded failure), and **failed** (last extract attempt for this upload failed). When failed, the response MAY include a short safe reason string that MUST NOT contain CV text or PII. The system SHALL expose an authenticated **retry parse** action that re-runs structured extraction for the currently stored résumé without requiring a re-upload.

#### Scenario: Pending status while stamp mismatches

- **WHEN** a résumé is stored and `resume_structured` is stamped to an older upload
- **THEN** résumé meta reports parse status pending (or failed if a failure was recorded for this upload)

#### Scenario: Retry parse without re-upload

- **WHEN** a signed-in user with a stored résumé invokes retry parse
- **THEN** the system attempts structured extraction for the current upload and, on success, stamps the structure current

#### Scenario: Failed status surfaces a safe reason

- **WHEN** the last extract for the current upload failed because the PII detector was unreachable
- **THEN** résumé meta reports failed with a short reason that does not include the CV body

## MODIFIED Requirements

### Requirement: The structured résumé is read-only and tied to the current résumé

The stored structured résumé SHALL remain read-only as an extract artifact — this capability provides no per-field editing of `resume_structured`. Candidate contact edits belong to the `candidate-contacts` capability, not this blob. The structure SHALL always describe the résumé currently stored for the user: a re-upload re-derives it, and it is **current** only when its stamp matches the current résumé's upload time. A structure whose stamp does not match MUST NOT be presented as the current full structured résumé on Profile semantic sections; seed composition MAY still read file-owned body fields from the last blob under `cv-tailoring` rules. Deleting the résumé SHALL clear the stored structured résumé.

Persisting the structure SHALL also derive and store the candidate's geography from the structure's location line, in the same statement and under the same stamp, so that a stored geography can never describe a different CV than the structure it was derived from. The derivation is deterministic and performs no I/O; the rules governing what it emits belong to the `candidate-geography` capability. Deleting the résumé SHALL clear the derived geography along with the structure. A successful persist SHALL also apply the extract fill-empty policy into candidate contacts.

#### Scenario: Re-upload re-derives the structure

- **WHEN** a user who already has a structured résumé uploads a new CV
- **THEN** the structured résumé is re-derived from the new CV in the background and, once persisted, replaces the previous one

#### Scenario: A structure from a superseded résumé is not served

- **WHEN** a newer résumé has been uploaded but its structured extraction has not yet completed
- **THEN** the read surface does not present the superseded structure as the current full structured résumé; parse status is pending or failed, and candidate contacts remain available separately

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

The system SHALL expose résumé status on the authenticated read (`GET /api/v1/me/resume`), including parse status, candidate contacts (or equivalent contact fields for Profile), and — when the structure stamp is current — the full sanitized structured résumé for semantic sections. When the stamp is not current, the response MUST NOT present superseded summary/education/skills as if they were current, except as allowed by seed rules elsewhere; it SHALL still return parse status and candidate contacts. The wire shape SHALL be generated to TypeScript via `cmd/gen-contracts`.

#### Scenario: Present structured résumé is returned

- **WHEN** a signed-in user with a current structured résumé requests their résumé status
- **THEN** the response includes the sanitized structured résumé alongside résumé metadata and parse status current

#### Scenario: Absent structured résumé is null

- **WHEN** a signed-in user has a résumé but no current structured résumé (unconfigured LLM, not yet extracted, or stale)
- **THEN** the response reports the current structured résumé as null, includes parse status pending or failed, returns candidate contacts when set, and the rest of the résumé status is unaffected

### Requirement: The profile page renders contacts editable and parse status visible

The web profile SHALL let the signed-in user edit candidate contacts (including links) without uploading a CV. It SHALL show parse status (and a retry control when pending or failed). Semantic sections from a **current** structured résumé (summary, education, languages, skills, certifications) MAY remain read-only displays of the extract. Experience continues to come from the experience bank.

#### Scenario: Profile edits links without upload

- **WHEN** a signed-in user corrects links on the Profile contacts editor and saves
- **THEN** the owned contacts update and no résumé upload is required

#### Scenario: Profile shows pending with retry

- **WHEN** parse status is pending or failed
- **THEN** the Profile tab shows that status and offers retry parse
