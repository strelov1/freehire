## ADDED Requirements

### Requirement: Populate profile skills from a resume

The search-profile create/edit form SHALL let a user upload a resume (PDF or pasted text) and
merge the extracted skills into the form's skills field. Extraction SHALL use the
`resume-skill-extraction` capability. Merging SHALL be a union with the skills already present
in the form (deduplicated); it SHALL NOT remove or overwrite skills the user already entered.
The user SHALL be able to edit or remove any skill chip before saving. Skills persist through
the existing profile create/update endpoints; no new persistence path is introduced.

#### Scenario: Merge extracted skills into an empty form

- **WHEN** a user with an empty skills field uploads a resume and extraction returns
  `[go, postgresql]`
- **THEN** the form's skills field contains exactly `[go, postgresql]`

#### Scenario: Merge extracted skills without wiping existing entries

- **WHEN** a user whose form already has `[docker]` uploads a resume returning `[go, docker, postgresql]`
- **THEN** the form's skills field contains the union `[docker, go, postgresql]` with no
  duplicates

#### Scenario: Extraction in progress shows a loading state

- **WHEN** the resume is being uploaded and analyzed
- **THEN** the upload control shows a loading/disabled state until extraction completes or fails

### Requirement: Display market skill-gap on a profile

Each saved profile that has at least one specialization SHALL display a skill-gap analysis
computed on the frontend against live market data. The frontend SHALL query
`GET /api/v1/jobs/facets` with the profile's specialization(s) as `category` filters (OR across
values), sort the returned `skills` facet by descending job count, and take the top N (N = 20)
as the expected skill set. Coverage SHALL be shown as `X/N`, where X is the count of expected
skills the profile already has. The skills in the expected set that the profile lacks SHALL be
shown as "missing" chips. The gap computation SHALL be a pure function
`computeGap(marketSkills, profileSkills, n)`.

#### Scenario: Coverage and missing skills for a profile

- **WHEN** a profile's specialization market top-20 skills are known and the profile has 13 of them
- **THEN** the card shows coverage `13/20` and lists the 7 missing skills as chips

#### Scenario: Multiple specializations combine markets

- **WHEN** a profile has specializations `[backend, devops]`
- **THEN** the expected skill set is derived from `/jobs/facets?category=backend&category=devops`
  (jobs in either category) taken as one combined top-20

#### Scenario: Profile without a specialization shows no gap block

- **WHEN** a profile has an empty specializations list
- **THEN** no skill-gap block is rendered for that profile

#### Scenario: Full coverage

- **WHEN** the profile already contains all top-20 expected skills
- **THEN** the card shows coverage `20/20` and lists no missing skills
