## ADDED Requirements

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
