# resume-verdict Specification (delta)

## ADDED Requirements

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

## MODIFIED Requirements

### Requirement: Verdict endpoint authentication and ownership

The verdict endpoint SHALL require an authenticated session (cookie-only) and
SHALL operate only on a profile owned by the caller. It SHALL be a single
read-only `GET /me/profiles/:id/verdict`. Requesting another user's profile, or a
missing profile, SHALL respond 404.

#### Scenario: Owner reads their verdict
- **WHEN** a signed-in user requests the verdict for a profile they own
- **THEN** the response is 200 with the verdict

#### Scenario: Non-owner is refused
- **WHEN** a signed-in user requests the verdict for a profile owned by someone else
- **THEN** the response is 404

## REMOVED Requirements

### Requirement: Deterministic market skill-gap scoring
**Reason**: Replaced by vacancy coverage — the verdict now measures the count and percent of open vacancies the profile's skills reach, not the share of the top-20 in-demand skills the profile holds.
**Migration**: Consume `covered`/`total`/`coverage_percent` instead of `stack_match` and the top-20 breakdown.

### Requirement: Per-gap unlock percentage
**Reason**: Replaced by per-skill new-vacancy unlock — a gap's value is now the number of currently-uncovered vacancies it unlocks, not its raw market share.
**Migration**: Consume each gap's `new_vacancies`/`unlock_percent`.

### Requirement: Must-have designation by demand share
**Reason**: The top-20 / must-have model is dropped; gaps are ranked by the new vacancies they unlock, so a demand-share threshold is no longer meaningful.
**Migration**: None — `must_have_total`/`must_have_covered` are removed from the response.

### Requirement: LLM coherence score and gap advice
**Reason**: The AI coherence feature is removed — it read as a disconnected second metric and required a second résumé upload on the verdict page.
**Migration**: None — `coherence` and per-gap `advice` are removed from the response; résumé→skills extraction remains available on the profile form.

### Requirement: Résumé text is never persisted
**Reason**: The verdict no longer ingests résumé text (coherence removed), so there is no verdict-side résumé text to govern.
**Migration**: None — the profile-form résumé→skills extraction governs its own transient handling under the resume-skill-extraction capability.

### Requirement: Graceful degradation without the LLM
**Reason**: The verdict has no LLM step; it is deterministic in all cases.
**Migration**: None — the endpoint always returns the deterministic coverage verdict (503 only when the search backend is unconfigured).
