## MODIFIED Requirements

### Requirement: The structured résumé is served on the résumé read surface

The system SHALL expose the structured résumé on the authenticated résumé status read (`GET /api/v1/me/resume`), so the profile UI can render the parsed sections. Work experience on that payload SHALL come from the experience bank whenever the bank is non-empty. File-owned semantic sections (summary, education, languages, skills, projects, certifications, years) SHALL come only from a structure whose stamp matches the current résumé upload; when that stamp is absent or stale those sections MUST be empty rather than taken from a superseded extract.

Contact fields (`full_name`, `email`, `phone`, `location`, `links`) on this **cookie-only** read are identity, not model output: when the structure stamp is current they SHALL come from that structure; when the stamp is stale or missing but a superseded structured blob still holds contacts, the read SHALL still populate those contact fields from that blob as **provisional** identity so the Profile tab does not render a career with a blank header. The response SHALL also carry an explicit signal that the file-owned parse is pending or stale whenever contacts are provisional or semantic sections are empty for that reason, so the UI can explain the gap rather than looking broken. The payload's `structured` object MAY be non-null when only banked experience (and optional provisional contacts) are present. The wire shape SHALL be generated to TypeScript via `cmd/gen-contracts`.

#### Scenario: Present structured résumé is returned

- **WHEN** a signed-in user with a current structured résumé requests their résumé status
- **THEN** the response includes the sanitized structured résumé (contacts and semantic sections from the current stamp, experience from the bank when populated) alongside the existing résumé metadata, and the pending/stale signal is clear

#### Scenario: Absent structured résumé is null

- **WHEN** a signed-in user has a résumé, an empty experience bank, and no current structured résumé (unconfigured LLM, not yet extracted, or stale) and no superseded structured blob with contacts
- **THEN** the response reports the structured résumé as null and the rest of the résumé status is unaffected

#### Scenario: Experience is served while the parse is pending

- **WHEN** an authenticated user's structured résumé stamp is stale but their bank is populated
- **THEN** the response still carries their banked work experience

#### Scenario: Provisional contacts survive the pending window

- **WHEN** a newer résumé has been uploaded, extraction has not yet completed, the bank is populated, and a superseded structured blob still holds name/email/phone/links
- **THEN** the résumé read carries those contact fields together with banked experience and an explicit pending/stale signal, rather than a contact-less experience-only shell

#### Scenario: No superseded contacts stay empty

- **WHEN** the structure stamp is stale, the bank is populated, and no structured blob (or an empty-contact blob) exists
- **THEN** experience is still served, contact fields remain empty, and the pending/stale signal is set so the UI can explain that identity is waiting on the parse

### Requirement: The profile page renders the structured résumé read-only

The web profile SHALL render the structured résumé's sections (experience, education, contacts, languages, links, summary) read-only when a structured payload is present. When the résumé read signals that the file-owned parse is pending or stale, the Profile tab SHALL show a clear pending explanation for missing semantic sections and SHALL still show provisional contacts and banked experience when the API provides them. When `structured` is null, the tab SHALL omit the parsed section without error (or show the existing empty-state copy), not a silent blank header under an experience list.

#### Scenario: Profile shows parsed sections

- **WHEN** a signed-in user with a current structured résumé opens their profile
- **THEN** the profile renders the parsed sections read-only, including name, phone, and links when present on the payload

#### Scenario: Profile omits the section when absent

- **WHEN** the user has no structured payload (`structured` is null)
- **THEN** the profile omits the structured section and shows no error

#### Scenario: Pending parse does not look like a broken parse

- **WHEN** the résumé read signals pending/stale and carries banked experience with provisional or empty contacts
- **THEN** the Profile tab shows that the latest CV is still being parsed (or failed to parse) rather than presenting an unlabeled empty name/phone/links area as if the CV had none
