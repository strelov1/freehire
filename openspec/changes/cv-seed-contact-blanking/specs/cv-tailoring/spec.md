## ADDED Requirements

### Requirement: A whole-document seed requires a current structured résumé

The composition that seeds a CV (experience bank plus the sections the stored structured résumé still owns) SHALL be treated as usable for creating or replacing a CV document only when a structured résumé current with the stored résumé is present. Banked work history alone MUST NOT make the seed usable: the bank is unstamped and additive, while contact fields, summary, education and the rest still come from the structured résumé and are absent when that structure is stale, pending, or never extracted.

When the seed is not usable, paths that would create a base CV or replace an existing document from the seed (first-time tailor bootstrap, stale-base refresh before a new tailored copy, reset-from-résumé) MUST refuse with a client error that tells the user to add or wait for a résumé parse, and MUST NOT write a document whose header was blanked by a structure-less composition.

#### Scenario: Bank-only composition is not a usable seed

- **WHEN** the experience bank has roles but no structured résumé is current with the uploaded résumé
- **THEN** the seed is not usable for whole-document create or replace

#### Scenario: Stale-base refresh does not blank the header

- **WHEN** the base CV predates a résumé upload whose structured extraction has not yet completed, and the candidate starts tailoring
- **THEN** the base CV's header is left unchanged and no blank-header document is written

#### Scenario: Reset-from-résumé refuses a structure-less seed

- **WHEN** the candidate requests reset-from-résumé while only banked experience is available
- **THEN** the request fails with a client error and the tailored CV's header is unchanged

### Requirement: Applying a seed never blanks existing contact fields

When a seed document is applied onto an existing CV (stale-base refresh or reset-from-résumé), each header contact field that is empty in the seed and non-empty on the existing document MUST keep the existing value. The seed MAY fill empty header fields and MAY replace a field when the seed carries a non-empty value; it MUST NOT erase identity the candidate already has on the page.

This is a defence in depth next to the usable-seed gate: even a partial structure that omitted one contact identifier must not wipe a value the candidate typed or that a previous extract filled.

#### Scenario: Empty seed email preserves the CV email

- **WHEN** a seed with an empty email is applied onto a CV whose header already has an email
- **THEN** the stored email is unchanged

#### Scenario: Non-empty seed phone replaces the CV phone

- **WHEN** a seed carrying a phone is applied onto a CV whose header has a different phone
- **THEN** the stored phone becomes the seed's phone

#### Scenario: Empty seed fills nothing that was already set

- **WHEN** a seed with empty name, email, phone, location and links is applied onto a CV with a filled header
- **THEN** every header contact field on the CV remains as it was
