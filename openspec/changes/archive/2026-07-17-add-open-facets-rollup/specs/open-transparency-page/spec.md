## MODIFIED Requirements

### Requirement: Public transparency page
The system SHALL serve a public, unauthenticated `/open` page that renders live
freehire metrics server-side (SSR), covering catalogue scale, catalogue movement,
facet distributions, open-source stats, and member growth.

#### Scenario: Page is public and server-rendered
- **WHEN** an anonymous visitor opens `/open`
- **THEN** the page responds 200 with the metrics present in the initial server-rendered HTML (no client-only data fetch required to see the figures)

#### Scenario: Catalogue scale section
- **WHEN** the page renders
- **THEN** it shows a stat-strip with the live open-job count, company count, the ATS-platform count, and the Telegram-channel count

#### Scenario: Catalogue movement section
- **WHEN** the page renders
- **THEN** it shows the added-vs-removed activity over time, reusing the same chart as `/trends` fed by `/api/v1/stats/jobs-activity`

#### Scenario: What's-inside section
- **WHEN** the page renders
- **THEN** it shows facet distributions (top countries, top skills, remote share, seniority split) derived from the precomputed `/api/v1/stats/facets` snapshot

#### Scenario: Member-growth section
- **WHEN** the page renders
- **THEN** it shows a cumulative member-growth chart fed by `/api/v1/stats/user-growth`

#### Scenario: Open-source section
- **WHEN** the page renders and the GitHub API is reachable
- **THEN** it shows the repository stars, forks, and contributor count, an MIT-license badge, and a contribute call to action

### Requirement: Figures link to their API source
The system SHALL link each headline figure to the public API endpoint that produced
it, reinforcing the API-first positioning.

#### Scenario: Stat links to endpoint
- **WHEN** a visitor inspects a headline figure (e.g. open jobs, member growth)
- **THEN** it links to the corresponding public API endpoint that returns that data

#### Scenario: What's-inside links to the facets snapshot endpoint
- **WHEN** a visitor inspects the "what's inside" section
- **THEN** its source link points to `/api/v1/stats/facets`
