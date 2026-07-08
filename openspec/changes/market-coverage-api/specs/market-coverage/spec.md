## ADDED Requirements

### Requirement: Stateless market-coverage from a supplied skill list

The system SHALL expose `POST /api/v1/market/coverage` that scores a caller-supplied
list of skills against the live open-vacancy market for a facet-filtered role, and
return the coverage result: the role's open-vacancy total, how many of those
vacancies list at least one of the supplied skills, the coverage percent, the
ranked missing-skill gaps (each with the new vacancies it would unlock), and the
role's top in-demand skills scored against the supplied skills.

The endpoint SHALL take the skills from the request body (`{"skills": [...]}`) and
the market filter from the request's facet query params. It SHALL reuse the same
coverage computation as the CV-based verdict (no divergent scoring logic).

#### Scenario: Coverage for a filtered role
- **WHEN** an authenticated caller posts `{"skills": ["go","docker"]}` with a facet
  filter (e.g. `?category=backend&seniority=senior`)
- **THEN** the response `data` reports `total`, `covered`, `coverage_percent`, a
  ranked `gaps` list, and a `skills` breakdown of the role's top in-demand skills
  with each skill's market frequency, must-have flag, and its status against the
  supplied skills (`strong` / `adjacent` / `missing`)

#### Scenario: Skills are the measured set, not a filter
- **WHEN** the caller supplies skills the market rarely lists
- **THEN** those skills narrow neither `total` nor the role query; they only change
  `covered` and the gap ranking (the market total reflects the facet filter alone)

### Requirement: Full facet-filter vocabulary

The endpoint SHALL accept the same complete facet vocabulary as the job search and
facets endpoints (every `search.StringFacets` param plus the numeric facets), so a
caller can narrow the market by any facet, not a fixed subset.

#### Scenario: Narrowing by an arbitrary facet
- **WHEN** the caller adds any supported facet param (e.g. `?countries=BR`,
  `?employment_type=full_time`, `?english_level=b2`, `?salary_min=100000`)
- **THEN** the coverage is computed against the market narrowed by that facet

### Requirement: Stateless response omits CV-section metrics

Because a flat skill list has no declared/body CV sections, the response SHALL NOT
report the CV-section metrics (`coherence`, and the `hidden` skill status). Skill
statuses SHALL be limited to `strong`, `adjacent`, and `missing`.

#### Scenario: No hidden status or coherence
- **WHEN** the coverage result is returned
- **THEN** no skill row carries the `hidden` status and the response does not
  advertise a coherence score

### Requirement: Authentication and precondition failures

The endpoint SHALL require authentication by session cookie or API key. It SHALL
return 400 when no skills are supplied (nothing to measure), 401 when the caller is
unauthenticated, and 503 when search is not configured.

#### Scenario: API-key caller
- **WHEN** a request carries a valid `Authorization: Bearer fhk_…` API key
- **THEN** the coverage is computed and returned

#### Scenario: Missing skills
- **WHEN** an authenticated caller posts an empty or absent `skills` list
- **THEN** the response is 400

#### Scenario: Unauthenticated caller
- **WHEN** a request carries no cookie and no API key
- **THEN** the response is 401

#### Scenario: Search unconfigured
- **WHEN** the search backend is not configured
- **THEN** the response is 503
