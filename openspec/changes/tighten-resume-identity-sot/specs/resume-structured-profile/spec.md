## MODIFIED Requirements

### Requirement: The structured résumé is read-only and tied to the current résumé

The stored structured résumé SHALL be read-only — this capability provides no per-field editing of extract fields. A re-upload re-derives it. Semantic sections (summary, headline, experience, education, skills, languages, projects, certifications, years) SHALL be served as “current structure” only when the structure stamp matches the current résumé's upload time.

A structured résumé whose stamp does not match the current résumé (a newer CV whose extraction has not yet landed, or a persistent extraction outage) MUST NOT be served as current structure. Identity fields from that superseded blob MAY be served as **provisional contacts** (name, email, phone, location, links only). Candidate-owned contacts are a separate store and are not cleared by this stamp rule.

Deleting the résumé SHALL clear the stored structured résumé and the derived geography. Deleting the résumé MUST NOT clear candidate-owned contacts.

Persisting the structure SHALL also derive and store the candidate's geography from the structure's location line, in the same statement and under the same stamp, so that a stored geography can never describe a different CV than the structure it was derived from. The derivation is deterministic and performs no I/O; the rules governing what it emits belong to the `candidate-geography` capability.

#### Scenario: Re-upload re-derives the structure

- **WHEN** a user who already has a structured résumé uploads a new CV
- **THEN** the structured résumé is re-derived from the new CV in the background and, once persisted, replaces the previous one

#### Scenario: A superseded résumé is not served as current structure

- **WHEN** a newer résumé has been uploaded but its structured extraction has not yet completed
- **THEN** the current-structure read reports absent/stale rather than the superseded semantic sections

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

The system SHALL expose résumé identity and parse state on the authenticated résumé status read (`GET /api/v1/me/resume`). The response SHALL distinguish:

- **current structure** — stamp matches; semantic sections may be present
- **provisional contacts** — stamp does not match; only identity fields from a superseded blob
- **candidate-owned contacts** — a separate field, preferred over extract contacts when set
- **parse status** — `pending`, `ok`, or `failed` for the current upload (empty when no résumé)

The composed `structured` object MAY include identity and banked experience/projects while extract is pending, but MUST omit superseded semantic sections (summary, education, skills, languages, certifications, years). The field SHALL be null when there is nothing worth serving (no owned contacts, no provisional identity, no current structure, no banked rows). The wire shape SHALL be generated to TypeScript via `cmd/gen-contracts`.

#### Scenario: Present structured résumé is returned

- **WHEN** a signed-in user with a current structured résumé requests their résumé status
- **THEN** the response includes the sanitized structured résumé alongside the existing résumé metadata and parse status `ok`

#### Scenario: Pending extract serves identity, not superseded body

- **WHEN** a signed-in user has a résumé whose extract stamp does not match the upload, and a superseded blob still holds a name and a summary
- **THEN** the response reports parse status `pending` (or `failed` if that is the recorded outcome), the composed structure may carry the name, and the superseded summary is absent

#### Scenario: Absent structured résumé is null when nothing else is worth serving

- **WHEN** a signed-in user has a résumé but no current structure, no provisional contacts, no owned contacts, and no banked rows
- **THEN** the response reports the structured résumé as null and the rest of the résumé status is unaffected

## ADDED Requirements

### Requirement: Three identity layers have one precedence

Résumé identity SHALL have exactly three layers, in this precedence for contact fields on Profile and CV seed/heal:

1. **Candidate-owned contacts** — written by the candidate (or filled empty-from a current extract). Survive résumé delete. When any owned field is set, that whole contact block wins over extract contacts.
2. **Current structured extract** — stamp matches the upload. Source of semantic sections and, when owned contacts are empty, of contact fields.
3. **Provisional contacts** — identity-only slice of a superseded blob while extract is pending or failed. Never a source of summary, education, skills, languages, projects, certifications, or years.

A reader that needs “is the file-owned parse current?” MUST use the stamp-gated current-structure read (false while pending). A reader that needs identity MUST use the precedence above. No fourth mix is allowed.

#### Scenario: Owned contacts overlay a current extract

- **WHEN** the candidate has set an owned email and the current extract has a different email plus a summary
- **THEN** Profile and seed composition show the owned email and still show the extract summary

#### Scenario: Owned contacts overlay provisional identity

- **WHEN** extract is pending, the superseded blob has a name, and owned contacts have a different name
- **THEN** Profile and seed composition show the owned name and do not show superseded semantic sections

#### Scenario: Stamp gate stays false while contacts are provisional

- **WHEN** the structure stamp does not match the current upload
- **THEN** the current-structure read still reports absent/stale even though provisional contacts are available for identity

### Requirement: Candidate-owned contacts survive résumé delete

The system SHALL persist candidate-owned contacts independently of the résumé object and of `resume_structured`. Deleting the stored résumé MUST leave those contacts in place. Re-upload MUST NOT wipe non-empty owned fields; a current extract MAY fill only empty owned fields. An explicit replace-from-CV action MAY overwrite owned contacts from the current extract.

#### Scenario: Delete keeps contacts

- **WHEN** a signed-in user who has owned contacts deletes their résumé
- **THEN** a subsequent résumé status read still returns those contacts

#### Scenario: Re-upload does not clobber a typed name

- **WHEN** the candidate has an owned full name and uploads a new CV whose extract has a different name
- **THEN** the owned full name is unchanged after the extract lands

#### Scenario: Replace-from-CV overwrites owned contacts

- **WHEN** the candidate requests replace-from-CV while a current structured résumé exists
- **THEN** owned contacts become the extract's contact fields
