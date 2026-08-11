## ADDED Requirements

### Requirement: Seed composition keeps summary, skills, and projects while structure is pending

When composing the seed document for creating or refreshing a base CV (first tailor, stale-base refresh, or reset from résumé), the system SHALL include:

- **Contacts** from candidate contacts (then current structure, then provisional contacts)
- **Experience** from the experience bank when it has jobs, else from structure
- **Projects** from bank project employments when present, else from structure projects (including the last stored structured blob when the stamp is not yet current)
- **Summary, skills, education, languages, certifications** from the current structure when stamped; when the stamp is pending or failed but a structured blob exists, from that blob's corresponding fields so a pending extract does not open a blank body

A provisional/pending composition MUST NOT blank summary, skills, or projects on an existing base when applying header-only heal. Full reseed from a pending composition MUST still apply the merge rules that preserve presentation (margins, template) and MUST NOT wipe candidate-owned header fields with empty seed contacts.

#### Scenario: Pending stamp still seeds summary and skills onto a new base

- **WHEN** a user with a stored résumé, a superseded structured blob that includes summary and skills, pending stamp, and no base CV requests tailoring
- **THEN** the seeded base CV includes that summary and those skills (and contacts from candidate contacts when set)

#### Scenario: Bank projects land on the tailored copy

- **WHEN** the experience bank holds a project employment and tailor creates a copy from a seeded base
- **THEN** the base and copy include that project under `projects[]`

#### Scenario: Header-only heal does not clear summary

- **WHEN** structure is pending and stale-base refresh only heals the header
- **THEN** the base CV's existing summary, skills, and projects remain unchanged
