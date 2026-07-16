## ADDED Requirements

### Requirement: Insights hub page

The system SHALL serve a server-rendered `GET /insights` hub page that links every
covered category and each insight type, so crawlers and users can reach the
per-category pages from one indexable entry point. The page SHALL be server-rendered
(content present in the initial HTML) and carry canonical, meta, and Open Graph tags.

#### Scenario: Hub lists covered categories

- **WHEN** a client requests `/insights`
- **THEN** the initial HTML contains links to each covered category's salary,
  skills, and roles pages, and no client-side fetch is required to see them

### Requirement: Per-category salary page

The system SHALL serve a server-rendered `GET /insights/salary/[category]` page
showing salary bands (percentiles) for the category broken down by seniority,
reported per currency. The page SHALL read the insights data server-side and render
it into the initial HTML.

#### Scenario: Salary page renders bands server-side

- **WHEN** a client requests `/insights/salary/backend` for a covered category
- **THEN** the response is `200` with the salary bands present in the server-rendered
  HTML, a data-driven intro sentence, and an "updated" date

#### Scenario: Uncovered category is not a live page

- **WHEN** a client requests `/insights/salary/<category>` for a category that does
  not clear the data-quality gate
- **THEN** the response is `404` (the page is not published) rather than a thin or
  empty page

### Requirement: Per-category skills page

The system SHALL serve a server-rendered `GET /insights/skills/[category]` page
ranking the most in-demand skills in that category with a growth measure, rendered
into the initial HTML.

#### Scenario: Skills page renders ranking server-side

- **WHEN** a client requests `/insights/skills/backend` for a covered category
- **THEN** the response is `200` with the ranked skills present in the server-rendered
  HTML

### Requirement: Per-category roles page

The system SHALL serve a server-rendered `GET /insights/roles/[category]` page
ranking the category's roles (its seniorities) by open-job demand with a growth
measure, rendered into the initial HTML.

#### Scenario: Roles page renders ranking server-side

- **WHEN** a client requests `/insights/roles/backend` for a covered category
- **THEN** the response is `200` with the ranked seniorities present in the
  server-rendered HTML

### Requirement: Data-quality gate for published pages

A category SHALL be published (given live pages and sitemap entries) only when its
insights data clears a configured threshold (e.g. a minimum number of open jobs, or
a salary band at/above the sample floor). Categories that do not clear the gate SHALL
NOT be linked from the hub, SHALL NOT appear in the sitemap, and their pages SHALL
return `404`. The set of covered categories SHALL be derived from live data, not
hard-coded.

#### Scenario: Thin category excluded everywhere

- **WHEN** a category's data is below the gate threshold
- **THEN** it is absent from the hub links and the sitemap, and its pages `404`

### Requirement: SEO content and structured data

Each insights page SHALL carry, server-rendered: a unique `<title>` and meta
description, a canonical URL, Open Graph tags, JSON-LD (at least BreadcrumbList and a
Dataset descriptor), and internal links to the relevant `/jobs` filtered view, to the
sibling categories, and to the other insight types for the same category.

#### Scenario: Structured data and internal links present

- **WHEN** any covered insights page is fetched
- **THEN** its HTML contains a canonical tag, JSON-LD with BreadcrumbList, and links
  to a `/jobs` filtered view plus the other insight types for the same category

### Requirement: Insights sitemap shard

The system SHALL expose a `sitemap-insights.xml` listing exactly the published
insights URLs (hub + covered categories × insight types), and SHALL reference this
shard from the sitemap index so search engines discover the pages.

#### Scenario: Sitemap lists only published pages

- **WHEN** `sitemap-insights.xml` is fetched
- **THEN** it lists the hub and every covered category's pages, and no uncovered
  (gated-out) category appears; the sitemap index references the shard
