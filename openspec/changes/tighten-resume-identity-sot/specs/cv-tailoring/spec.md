## ADDED Requirements

### Requirement: Seed composition follows the identity table and splits jobs from projects

The composition that seeds a CV SHALL assemble:

- **Contacts** from the three-layer identity table (owned, then current extract, then provisional).
- **Semantic sections** (summary, education, skills, languages, certifications, years) from the **current** structured résumé only. A superseded blob MUST NOT contribute those sections.
- **Work history** from banked job-kind employments when the bank has any employment; otherwise from the current structure's experience (empty-bank fallback). A populated bank MUST NOT be merged with structure experience.
- **Projects** from banked project-kind employments when any exist; otherwise from the current structure's projects. Project rows MUST land in the document's projects section (name, optional link, publishable bullets), not as nameless job rows.

The composition is **usable** for whole-document create or replace when it carries any seedable field (contacts, summary, headline, experience, education, skills, languages, projects, or certifications). Banked work history alone with empty identity MUST NOT be treated as usable: that composition blanks the header.

When the seed is not usable, paths that would create a base CV or replace an existing document from the seed MUST refuse with a client error (or leave the document unchanged for best-effort stale-base refresh), and MUST NOT write a document whose header was blanked by a structure-less composition.

#### Scenario: Provisional contacts plus bank make a usable seed

- **WHEN** the structure stamp is stale, owned or provisional contacts include a name, and the experience bank has job-kind roles
- **THEN** the seed is usable, the seeded header carries those contacts, experience comes from the bank, and superseded summary/education stay off the document

#### Scenario: Empty-bank fallback uses current structure jobs only

- **WHEN** the bank has no employments and a current structured résumé lists jobs and a summary
- **THEN** the seed's experience and summary come from that current structure

#### Scenario: Bank-only without identity is not usable

- **WHEN** the experience bank has roles but owned contacts are empty, no current structure exists, and no provisional contacts exist
- **THEN** the seed is not usable for whole-document create or replace

#### Scenario: Populated bank ignores structure experience

- **WHEN** the bank holds job-kind employments and the current structured résumé still lists a different set of jobs
- **THEN** the seed's experience comes only from the bank's job-kind employments

### Requirement: Reset and heal use opposite header merges

Reset-from-résumé SHALL apply a **seed-first** header merge: a non-empty seed contact field replaces the CV field; an empty seed field keeps the existing CV field. Opening a CV to heal SHALL apply a **keep-first** fill: only empty CV header fields are copied from identity; non-empty CV fields stay put. Both paths MUST leave body sections, template, and typography untouched by the header merge itself. Reset still replaces body sections from the seed document.

Stale-vs-upload refresh of the base CV SHALL full-reseed only when a current structure is available. While extract is pending or failed, that path MUST heal the header only — it MUST NOT run a whole-document seed that would blank summary and education the candidate already has on the base.

#### Scenario: Reset replaces a name the seed carries

- **WHEN** the candidate resets from résumé and the usable seed has a different full name than the CV header
- **THEN** the stored header name becomes the seed name

#### Scenario: Reset keeps a phone the seed left empty

- **WHEN** the candidate resets from résumé and the seed has an empty phone while the CV header already has a phone
- **THEN** the stored phone is unchanged

#### Scenario: Pending extract refreshes header only

- **WHEN** the base CV predates a résumé upload whose extract has not landed, provisional or owned contacts exist, and the candidate starts tailoring
- **THEN** empty header fields on the base are filled from identity and the base body is not replaced by a contacts-only seed
