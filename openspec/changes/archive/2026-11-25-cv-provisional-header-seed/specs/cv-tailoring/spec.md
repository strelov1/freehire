## ADDED Requirements

### Requirement: Whole-document seed admits provisional contacts while extract is pending

The composition that seeds a CV (experience bank plus the sections a **current** structured résumé still owns) SHALL be usable for creating or replacing a CV document when either:

1. a structured résumé current with the stored upload is present and the composition is non-empty, or
2. no current structure is available, but provisional contact fields from a superseded structured blob are present **and** the composition still carries seedable body content (typically banked experience) and/or a non-empty provisional name.

When usable via provisional contacts, the seed SHALL copy those contact fields into the CV header and SHALL use banked experience (and other bank-owned sections) as today, and MUST NOT copy superseded semantic sections (summary, education, languages, skills, projects, certifications, years) from the stale blob.

Banked work history alone with empty provisional contacts MUST NOT make the seed usable: that composition blanks the header and MUST NOT be written.

When the seed is not usable, paths that would create a base CV or replace an existing document from the seed MUST refuse with a client error (or leave the document unchanged for best-effort stale-base refresh), and MUST NOT write a document whose header was blanked by a structure-less composition.

#### Scenario: Provisional contacts plus bank make a usable seed

- **WHEN** the structure stamp is stale, a superseded blob still holds name and email, and the experience bank has roles
- **THEN** the seed is usable, the seeded header carries those contacts, and experience comes from the bank

#### Scenario: Superseded semantic sections stay out of the seed

- **WHEN** a stale structured blob still has a summary and education alongside contacts
- **THEN** a provisional seed does not place that summary or education onto the CV

#### Scenario: Bank-only without provisional contacts is not usable

- **WHEN** the experience bank has roles but no current structure and no provisional contacts
- **THEN** the seed is not usable for whole-document create or replace

#### Scenario: Stale-base refresh fills an empty header from provisional contacts

- **WHEN** the base CV predates a résumé upload, its header is empty, provisional contacts exist, and the candidate starts tailoring
- **THEN** the base CV header is filled from provisional contacts before the tailored copy is taken from it

### Requirement: Opening a tailored CV heals an empty header from provisional contacts

When an authenticated owner loads a tailored CV (read or tailor bootstrap that returns the existing vacancy-bound copy) and every header contact field on that document is empty, the system SHALL fill empty header fields from provisional contacts when those are available, using the same empty-seed merge rule (never overwrite a non-empty field), and SHALL persist the healed document so subsequent loads keep the contacts. If provisional contacts are absent, the document is left unchanged.

The heal MUST NOT rewrite body sections, template, or typography. It MAY also heal the owner's base CV header under the same empty-header + provisional-contacts conditions so future tailored copies do not reintroduce a blank header.

#### Scenario: Empty tailored header is filled on open

- **WHEN** the owner opens a tailored CV whose header is entirely empty and provisional contacts exist
- **THEN** the returned document's header carries those contacts and the stored row is updated

#### Scenario: Partial header is not wiped

- **WHEN** the owner opens a tailored CV that already has a name but empty email, and provisional contacts include both
- **THEN** the name is unchanged and the empty email is filled from provisional contacts

#### Scenario: No provisional contacts leaves the empty header alone

- **WHEN** the owner opens a tailored CV with an empty header and no provisional contacts are available
- **THEN** the stored document is unchanged
