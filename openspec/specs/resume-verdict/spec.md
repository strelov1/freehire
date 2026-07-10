# resume-verdict Specification

## Purpose
TBD - created by syncing change resume-verdict. Update Purpose after archive.
## Requirements
### Requirement: Verdict endpoint authentication and ownership

The profile sub-resource endpoints SHALL require an authenticated session
(cookie-only) and SHALL operate only on the caller's own single profile. These
endpoints are the market-coverage verdict and the CV ATS-readiness report, and
SHALL be addressed without a profile id in the path: the verdict as
`GET /me/profile/verdict`, the ATS report as `GET`/`POST /me/profile/ats-report`.
When the caller has no profile, these endpoints SHALL respond 404.

#### Scenario: Owner reads their verdict
- **WHEN** a signed-in user who has a profile requests `GET /me/profile/verdict`
- **THEN** the response is 200 with the verdict for their profile

#### Scenario: Owner reads their ATS report
- **WHEN** a signed-in user who has a profile requests `GET /me/profile/ats-report`
- **THEN** the response is 200 with the report for their profile

#### Scenario: No profile yet
- **WHEN** a signed-in user who has not saved a profile requests `GET /me/profile/verdict` or `GET /me/profile/ats-report`
- **THEN** the response is 404

#### Scenario: Unauthenticated request refused
- **WHEN** a request without a valid session cookie hits `GET /me/profile/verdict` or `/me/profile/ats-report`
- **THEN** the response is 401

### Requirement: Vacancy coverage of the selected role

The system SHALL compute a verdict for a search profile as the coverage its saved
skills achieve over the live market for the selected role(s): the count of open
vacancies that list at least one of the profile's skills, out of the total open
vacancies for the role. The role is the set of selected specialization
categories (OR-combined). Coverage SHALL be reported as an absolute `covered`
count, a `total` count, and a `coverage_percent` = round(covered / total × 100).
When the role has no open vacancies (`total` = 0), `coverage_percent` SHALL be 0.

#### Scenario: Coverage reported as count and percent
- **WHEN** a role has 1000 open vacancies and 630 of them list at least one of the profile's skills
- **THEN** the verdict reports `total` = 1000, `covered` = 630, and `coverage_percent` = 63

#### Scenario: Role with no open vacancies
- **WHEN** the selected role has 0 open vacancies
- **THEN** the verdict reports `total` = 0, `covered` = 0, and `coverage_percent` = 0

#### Scenario: No market data available
- **WHEN** the search/facet backend is not configured
- **THEN** the verdict endpoint responds 503 and no verdict is produced

### Requirement: Per-skill new-vacancy unlock

For each in-demand skill the profile lacks, the system SHALL report how many
currently-uncovered vacancies list that skill — i.e. vacancies in the role that
list the skill but none of the profile's current skills (`new_vacancies`) — and
`unlock_percent` = round(new_vacancies / total × 100). Gaps SHALL be ranked by
`new_vacancies` descending, breaking ties by ascending skill slug, and the
response SHALL carry at most the top 20. Skills the profile already has SHALL NOT
appear as gaps.

#### Scenario: Gap carries the new vacancies it unlocks
- **WHEN** 190 open vacancies in a 1000-vacancy role list "kubernetes" and none of those 190 list any skill the profile already has
- **THEN** the gap row for "kubernetes" reports `new_vacancies` = 190 and `unlock_percent` = 19

#### Scenario: A skill covered by existing skills is not double-counted
- **WHEN** a vacancy lists both "kubernetes" (a gap) and "docker" (a skill the profile has)
- **THEN** that vacancy is already counted as covered and does NOT contribute to "kubernetes"'s `new_vacancies`

#### Scenario: Owned skills are not gaps
- **WHEN** the profile already lists "go"
- **THEN** "go" does not appear in the gap list regardless of its market demand

#### Scenario: Gaps ranked biggest win first
- **WHEN** the uncovered market has "kafka" unlocking 120 vacancies and "grpc" unlocking 64
- **THEN** "kafka" is ranked before "grpc" in the gap list

### Requirement: Interactive role and filter selection

The verdict endpoint SHALL accept the same facet query parameters as the job
search (e.g. `category`, `seniority`, `regions`), letting the caller recompute
coverage for an ad-hoc role without modifying the stored profile. When no
`category` parameter is supplied, the calculation SHALL default to the profile's
own specializations. The profile's skills are always the measured set and SHALL
never be taken from the filter parameters.

#### Scenario: Filter overrides the profile's role
- **WHEN** the caller requests a profile's verdict with `?category=data&seniority=senior`
- **THEN** coverage is computed over senior data-category vacancies, and the stored profile is unchanged

#### Scenario: Defaults to the profile's specializations
- **WHEN** the caller requests a profile's verdict with no `category` parameter
- **THEN** coverage is computed over the union of the profile's saved specializations

#### Scenario: Profile skills are not a filter
- **WHEN** the caller passes `?skills=rust` while the profile lists "go"
- **THEN** coverage still measures the profile's "go" skill and "rust" is not treated as an owned skill

### Requirement: Profile summary shows headline coverage

The profile list SHALL show each profile's headline coverage — the `covered`
count and `coverage_percent` over the profile's own specializations — so the user
sees the key number without opening the detailed verdict.

#### Scenario: Coverage headline on the profile card
- **WHEN** the profile list renders a profile whose skills cover 630 of 1000 role vacancies
- **THEN** the card shows the coverage as 630 and 63%

### Requirement: Transferable/adjacent skill status

The verdict SHALL classify a top-20 role skill the CV does not hold exactly (neither
`strong` nor `hidden`) as `adjacent` when the CV holds a skill listed as adjacent to
it in a curated adjacency dictionary; otherwise it remains `missing`. Classification
precedence SHALL be `strong` → `hidden` → `adjacent` → `missing`. An `adjacent`
skill SHALL NOT count toward must-have coverage or stack-match (the exact skill is
absent) — it is surfaced as a close, reframe-able gap rather than a hard miss. The
classification SHALL be deterministic and require no LLM.

#### Scenario: Adjacent when a close skill is held
- **WHEN** the role wants "rest-apis", the CV lacks it but lists "fastapi", and the adjacency dictionary maps rest-apis → {fastapi}
- **THEN** the skill's status is `adjacent`

#### Scenario: Missing when no close skill is held
- **WHEN** the role wants "rust", the CV holds no skill adjacent to rust
- **THEN** the skill's status is `missing`

#### Scenario: Adjacent does not inflate coverage
- **WHEN** a must-have role skill is `adjacent` (not strong or hidden)
- **THEN** it does not count toward `must_have_covered` or `stack_match_percent`

### Requirement: Typed, actionable per-skill advice

Each non-`strong` skill status SHALL carry a concrete, status-specific advice line:
`adjacent` SHALL name the close skill the candidate holds and prompt reframing or
ramp-up; `hidden` SHALL prompt surfacing the skill in the Skills section; `missing`
SHALL prompt learning it and evidencing it. `strong` SHALL carry no advice. Advice
SHALL be deterministic templates.

#### Scenario: Adjacent advice names the held skill
- **WHEN** "rest-apis" is `adjacent` because the CV lists "fastapi"
- **THEN** its advice references "fastapi" as the close skill to reframe around

### Requirement: Skill-bundle rows on the verdict

The verdict SHALL include the CV's coverage of the curated skill bundles (see the
skill-bundles capability) so the candidate sees which market skill *combinations*
they cover, not only isolated skills.

#### Scenario: Bundle coverage carried on the verdict
- **WHEN** the CV covers the `genai-core` and `cloud-ops` bundles but not `web-stack`
- **THEN** the verdict's bundle rows report genai-core and cloud-ops as covered and web-stack as not covered

