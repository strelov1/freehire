## ADDED Requirements

### Requirement: Market-anchored role-skill breakdown

The verdict SHALL include a breakdown of the selected role's top 20 in-demand
skills (the role's `skills` facet, ranked by vacancy frequency). For each skill it
SHALL report: `name`, `market_frequency` = round(vacancies listing it / role total
× 100), a `must_have` flag (true when `market_frequency` ≥ a configured threshold),
a `status`, and an `advice` line. `status` SHALL be derived from the CV's parsed
skill sets (see cv-section-parsing): `strong` when the skill is in `declared`,
`hidden` when it is in `body` but not `declared`, `missing` when it is in neither.
`advice` SHALL be a deterministic status-specific line (empty for `strong`). Every
number SHALL come from live market data and the CV text — never from an LLM.

#### Scenario: Strong when declared in the Skills section
- **WHEN** a role's top skill "go" is present in the CV's `declared` set
- **THEN** its row has `status` = `strong` and no advice

#### Scenario: Hidden when used in experience but not declared
- **WHEN** a role's top skill "kafka" is in the CV's `body` but not `declared`
- **THEN** its row has `status` = `hidden` with advice to surface it in the Skills section

#### Scenario: Missing when absent from the CV
- **WHEN** a role's top skill "rust" is in neither `declared` nor `body`
- **THEN** its row has `status` = `missing` with advice to gain and evidence it

#### Scenario: Must-have flagged by market frequency
- **WHEN** "python" appears in 62% of the role's open vacancies and the threshold is 50%
- **THEN** its row has `must_have` = true

#### Scenario: Rare skill is not must-have
- **WHEN** "cobol" appears in 3% of the role's open vacancies and the threshold is 50%
- **THEN** its row has `must_have` = false

### Requirement: Market-anchored headline stats

The verdict SHALL report three additional headline stats alongside the existing
vacancy coverage: `must_have_total` and `must_have_covered` (of the role's
must-have skills, how many the CV demonstrably holds — `strong` or `hidden`);
`stack_match_percent` = round((`strong` + `hidden` among the top 20) / 20 × 100);
and `coherence_percent` = round(|`declared` ∩ `body`| / |`declared`| × 100), which
is 0 when `declared` is empty. These SHALL be computed deterministically from the
role facets and the CV's parsed skill sets.

#### Scenario: Must-have coverage counts strong and hidden
- **WHEN** the role has 7 must-have skills and the CV holds 6 of them as `strong` or `hidden`
- **THEN** `must_have_total` = 7 and `must_have_covered` = 6

#### Scenario: Stack match is top-20 breadth
- **WHEN** 15 of the role's top 20 skills are `strong` or `hidden` in the CV
- **THEN** `stack_match_percent` = 75

#### Scenario: Coherence penalizes unbacked declared skills
- **WHEN** the CV declares 10 skills and only 7 of them also appear in the body
- **THEN** `coherence_percent` = 70

#### Scenario: Coherence with no declared skills
- **WHEN** the CV has no Skills section (`declared` is empty)
- **THEN** `coherence_percent` = 0

### Requirement: Unified profile and coverage page

The signed-in profile SHALL live on a single `/my/profile` page that combines inline profile editing, market coverage, and CV readiness; the separate `/my/profile/verdict` route SHALL be removed and its content folded into this page. The page SHALL edit the profile inline (no modal): a CV drop-zone, a skills selector, and a specializations selector, saved together by an explicit Save action. A left filter sidebar (the same `/jobs` filter surface, with the `skills` facet excluded) SHALL scope the market comparison by role and other facets; its role selection SHALL be seeded from the profile's specializations but SHALL be independent of the saved profile, so refining the comparison role never mutates the profile.

#### Scenario: Setup shows only the form
- **WHEN** a signed-in user with no saved profile opens `/my/profile`
- **THEN** the page shows the inline editing form (with the CV upload) and no coverage or CV-readiness tabs

#### Scenario: Editing is inline, not a modal
- **WHEN** a user with a saved profile changes their skills or specializations
- **THEN** they edit the fields directly on the page and persist them with a single Save action — no separate edit modal is opened

#### Scenario: Filter refines comparison without touching the profile
- **WHEN** the user changes the sidebar role/facet filter
- **THEN** the coverage and CV-readiness numbers recompute for the filtered market while the saved profile's specializations are unchanged

#### Scenario: CV already uploaded is indicated
- **WHEN** the user has a stored CV (`has_cv` is true)
- **THEN** the CV drop-zone shows an "uploaded" state offering to update it, rather than an empty upload prompt

#### Scenario: Results are tabbed
- **WHEN** a user with a saved profile views the page
- **THEN** market coverage and CV readiness are presented as two tabs under the editing form
