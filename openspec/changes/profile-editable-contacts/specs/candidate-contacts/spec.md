## Purpose

Candidate-owned contact identity (name, email, phone, location, links) that Profile can edit without a CV re-upload, and that CV seed/heal prefer over a pending or empty extract.

## ADDED Requirements

### Requirement: Candidate contacts are stored separately from the structured résumé

The system SHALL persist a per-user **candidate contacts** record (full name, email, phone, location, links) that is independent of `resume_structured`. The structured résumé blob remains an extract artifact and MUST NOT become the write target for Profile contact edits. Deleting the stored résumé MUST NOT clear candidate contacts.

#### Scenario: Contacts survive résumé delete

- **WHEN** a signed-in user deletes their stored résumé after editing contacts
- **THEN** their candidate contacts remain available on the next Profile read

#### Scenario: Contact edits do not rewrite resume_structured

- **WHEN** a signed-in user updates their links via the contacts write API
- **THEN** `resume_structured` is unchanged

### Requirement: Candidate contacts are readable and writable on an authenticated surface

The system SHALL expose authenticated read and write of candidate contacts (cookie session for writes). A successful write SHALL return the stored contacts. Validation SHALL bound lengths and link cardinality consistently with CV header sanitization. Empty fields are allowed.

#### Scenario: Owner updates links without uploading a CV

- **WHEN** a signed-in user PUTs a contacts payload with corrected links and no résumé upload
- **THEN** the contacts are persisted and subsequent reads return those links

#### Scenario: Invalid contact payload is refused

- **WHEN** a contacts write exceeds length or link cardinality bounds
- **THEN** the request is refused and stored contacts are unchanged

### Requirement: Extract fills empty candidate contacts only

When a structured résumé extract completes successfully, the system SHALL copy contact fields from the extract into candidate contacts **only where the owned field is empty**. Non-empty owned fields MUST NOT be overwritten by extract. An explicit user action "replace contacts from CV" MAY overwrite from the current structure when one exists.

#### Scenario: Hand-edited link survives a later extract

- **WHEN** the user has set links on candidate contacts and a new extract completes with different links
- **THEN** the owned links remain as the user set them

#### Scenario: Empty owned email is filled from extract

- **WHEN** candidate contacts have an empty email and a successful extract includes an email
- **THEN** the owned email becomes that extracted value

### Requirement: CV seed and header heal prefer candidate contacts

When composing a CV seed or healing an empty CV header, the system SHALL take contact fields from candidate contacts when present, falling back to current structured contacts, then provisional extract contacts. A seed MUST NOT blank a CV header field solely because the structure stamp is pending if candidate contacts supply that field.

#### Scenario: Pending structure still seeds owned name and links

- **WHEN** structure is pending and candidate contacts hold a name and links
- **THEN** a base CV seed or header heal includes that name and those links
