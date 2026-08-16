## MODIFIED Requirements

### Requirement: Public transparency page
The system SHALL serve a public, unauthenticated `/open` page that renders live
freehire metrics server-side (SSR), covering catalogue scale, catalogue movement,
facet distributions, open-source stats, and member growth.

Every figure in the catalogue-scale strip SHALL come from the catalogue-scale
snapshot endpoint. The page SHALL NOT carry the ATS-platform count or the
Telegram-channel count as frontend constants, and SHALL NOT read the open-job
and company counts as two separate list totals — a single snapshot response
supplies all four, so the strip cannot show figures taken at different moments.

#### Scenario: Page is public and server-rendered
- **WHEN** an anonymous visitor opens `/open`
- **THEN** the page responds 200 with the metrics present in the initial server-rendered HTML (no client-only data fetch required to see the figures)

#### Scenario: Catalogue scale section
- **WHEN** the page renders
- **THEN** it shows a stat-strip with the live open-job count, company count, the ATS-platform count, and the Telegram-channel count, all four taken from one catalogue-scale snapshot

#### Scenario: Platform and channel counts track the backend
- **WHEN** a source adapter or a crawled Telegram channel is added or removed
- **THEN** the `/open` stat-strip reflects it on the next snapshot with no frontend change

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
