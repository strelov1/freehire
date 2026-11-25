# cv-tailoring-seed-reset Specification

## Purpose
How a tailored copy is seeded, aligned to its vacancy, and rebuilt by reset or refresh. Split out of cv-tailoring.

## Requirements

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

### Requirement: Tailoring seed maps projects and certifications without collapsing projects into jobs

The shared seed composition used by first-time tailor bootstrap, stale-base refresh, and reset-from-résumé SHALL place portfolio projects in the document's projects section (name, optional link, publishable bullets) and SHALL map structured-résumé certifications into the document's certifications section. Project-kind bank rows MUST NOT be written only as experience rows that drop the project URL. Usable-seed and contact-merge rules from the contact-blanking change remain in force.

#### Scenario: Bootstrap keeps project links

- **WHEN** a first-time user with a structured résumé that lists a linked portfolio project and a bank that imported that project requests tailoring
- **THEN** the seeded base CV's projects section includes that project name and URL

#### Scenario: Bootstrap includes certifications

- **WHEN** a first-time user whose structured résumé lists certifications requests tailoring
- **THEN** the seeded base CV includes those certifications

#### Scenario: Reset-from-résumé restores projects and certs

- **WHEN** the candidate resets a tailored CV from the résumé while the bank and structure carry projects and certifications
- **THEN** the applied document gains those projects (with links) and certifications rather than leaving them empty

### Requirement: Empty-bank experience fallback applies on the tailor seed path

When composing a tailor/bootstrap seed, if the experience bank has no employments but the current structured résumé still carries experience, the seed MUST use that structure's experience for the work-history section. Once the bank holds any employment, structure experience MUST NOT be merged back in (deleted bank roles stay deleted).

#### Scenario: Pending bank still seeds jobs from the structure

- **WHEN** a candidate with a current structured résumé that lists jobs, but whose bank import has not yet produced employments, starts tailoring with no base CV
- **THEN** the seeded base CV's experience comes from the structured résumé

#### Scenario: Populated bank ignores structure experience

- **WHEN** the bank holds employments and the structured résumé still lists a different set of jobs
- **THEN** the seed's experience comes only from the bank's job-kind employments

### Requirement: New tailored copies are surface-aligned to the vacancy

When the system creates a new tailored CV for a vacancy, it SHALL apply the
jd-surface-align rewrite to that copy against the vacancy's description before
the copy is stored and before any model turn on the copy. A bootstrap that
returns an already-existing tailored CV SHALL NOT re-apply alignment merely
because bootstrap was repeated.

#### Scenario: Fresh copy uses the vacancy's skill wording

- **WHEN** bootstrap creates a tailored copy and the base CV's skills or prose use
  an alias of a skill the vacancy names under a different surface form
- **THEN** the tailored copy is stored with the vacancy's preferred surface forms
  already applied

#### Scenario: Reload does not fight user edits

- **WHEN** bootstrap is repeated for a vacancy that already has a tailored copy
- **THEN** alignment is not re-run as part of that repeat

### Requirement: Reset-from-résumé re-aligns the rebuilt copy

When a tailored CV is reset from the résumé seed, the system SHALL apply the
jd-surface-align rewrite to the rebuilt document against the bound vacancy
before the reset is considered complete. The rewrite SHALL be committed through
the same editor path as other document updates.

#### Scenario: Reset copy speaks the vacancy's wording

- **WHEN** the owner resets a tailored CV whose seed again says `IaC` and the
  vacancy prefers `infrastructure as code`
- **THEN** the stored document after reset uses the vacancy's preferred surface

### Requirement: Refresh from tailor context uses reset-from-résumé

When the candidate agrees to refresh CV content after a bank edit while a tailored CV is
open in the tailor workspace, the system SHALL apply the same reset-from-résumé operation
already used by History → Reset Changes for that CV id (including base refresh as a side
effect of that endpoint, keep-if-empty summary/skills rules, and vacancy surface-align on
the tailored copy). The system MUST NOT introduce a separate silent rewrite path.

#### Scenario: Agreeing on tailor calls reset for the open CV

- **WHEN** the tailor workspace has an open tailored CV and the candidate agrees to refresh
  after a bank edit
- **THEN** `POST /api/v1/me/cvs/:id/reset-from-resume` runs for that id and the stored
  document reflects the current seed

### Requirement: Profile refresh reseeds the base CV with consent

When the candidate agrees to refresh from the profile Experience tab, the system SHALL
reseed the owner's base CV from the current seed composition (bank + current structured
résumé rules), preserving presentation. If the owner has no base CV, the system MAY create
one from the seed. The system MUST NOT require opening a tailored CV solely to refresh the
base.

#### Scenario: Base reseed endpoint (or equivalent) updates the base

- **WHEN** the owner confirms refresh on profile and a base CV exists
- **THEN** the base document body matches the current seed (with keep-if-empty for empty
  seed summary/skills) and title/template/margins/style are unchanged

### Requirement: Reset preserves skills and summary when the seed omits them

When the owner resets a tailored CV from the résumé seed, the system SHALL apply
seeded body content to the tailored document (and to the base refresh that shares the
same apply path). When the composed seed's summary is empty, the system SHALL keep the
document's existing summary. When the composed seed's skills section is empty, the
system SHALL keep the document's existing skills. The system MUST NOT copy superseded
structured summary or skills into the seed while the structured stamp is pending or
stale — keep-if-empty applies only to what is already stored on the CV.

#### Scenario: Pending extract with bank experience keeps prior skills and summary

- **WHEN** the structured stamp is not current (provisional contacts only), the experience
  bank has roles so reset is allowed, and the tailored CV already has a summary and skills
- **THEN** after reset the tailored CV still has that summary and those skills, and the
  experience sections reflect the bank seed

#### Scenario: Empty seed skills do not wipe tailored skills

- **WHEN** a current structured extract has no skills list but the tailored CV has skill chips
- **THEN** after reset the tailored CV's skill chips remain

### Requirement: Reset applies current extract skills and summary

When the composed seed carries a non-empty summary or skills (from a current structured
résumé), reset-from-résumé SHALL write those values onto the tailored document, replacing
whatever summary or skills were there. Seed wins when present.

#### Scenario: Current extract lands skills and summary on reset

- **WHEN** the structured stamp is current, the extract includes a summary and skills, and
  the owner resets a tailored CV
- **THEN** the stored tailored document's summary and skills match the seed (after any
  vacancy surface-align on the tailored copy alone)

### Requirement: Seeded base CVs place portfolio work in projects

When the system seeds or refreshes a base CV from the structured résumé and/or
the experience bank, portfolio and personal projects SHALL land in
`document.projects`, not in `document.experience`. A base CV whose sources
include such projects MUST NOT persist with an empty `projects` array while
those entries exist only under experience or only on a tailored copy.

#### Scenario: Bank project becomes a CV project on seed

- **WHEN** the experience bank holds a `kind=project` employment and a base CV is seeded or refreshed from the bank
- **THEN** that entry appears under `projects` on the base document and not as an `experience` role

#### Scenario: Structure projects become CV projects on seed

- **WHEN** the current structured résumé lists projects and a base CV is seeded from that structure
- **THEN** those entries appear under `projects` on the base document

