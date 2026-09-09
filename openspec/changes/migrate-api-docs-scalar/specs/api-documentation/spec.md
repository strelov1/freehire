## MODIFIED Requirements

### Requirement: Single typed source of truth for API docs

The system SHALL describe the public API as typed data in a single module
(`web/src/lib/docs/api-spec.ts`) from which the rendered page, a generated
OpenAPI document, and the `docs/API.md` file are all produced, so the three
representations cannot drift.

#### Scenario: One source feeds both outputs

- **WHEN** an endpoint or parameter is added or edited in `api-spec.ts`
- **THEN** the generated OpenAPI document reflects it on next generation, the
  rendered `/docs/api` page (which renders that generated OpenAPI document)
  reflects it once regenerated, and re-running the docs generator updates
  `docs/API.md` from the same data — with no separate hand-edit of any of
  the three

#### Scenario: Filter vocabulary derives from generated contracts

- **WHEN** the documented job-search filter table is built
- **THEN** its facet values come from `web/src/lib/generated/contracts.ts` and
  `web/src/lib/facets.ts` (the existing source of truth mirrored from Go
  `StringFacets`), not a hand-maintained duplicate list

### Requirement: Documented API coverage

The documentation SHALL cover the whole public API surface: the base URL, the
response envelope and pagination conventions, the public job reads
(`/jobs`, `/jobs/search`, `/jobs/facets`, `/jobs/:slug`, `/jobs/:slug/similar`),
companies, authentication, API keys, per-user job interactions, submissions,
reports, and saved searches/subscriptions. Each endpoint SHALL state its method,
path, authentication requirement, parameters, and a copyable curl example,
rendered via the embedded OpenAPI reference.

`web/static/openapi.yaml` is the integration contract, so every endpoint that
declares `experience_years_min` SHALL also declare its companion
`experience_years_max`. The two SHALL be documented as a pair whose meaning is a
range over the posting's stated experience requirement, and the documentation SHALL
state that either bound excludes postings that state no requirement.

#### Scenario: Endpoint entry is complete

- **WHEN** the documentation lists an endpoint
- **THEN** it shows the HTTP method, the path, an authentication badge (public /
  session-or-key / session-only / moderator / browser-extension-only), its
  parameters, and a copyable curl example

#### Scenario: Deprecated endpoint is marked with its replacement

- **WHEN** an endpoint in `api-spec.ts` is marked deprecated with a replacement
- **THEN** the documentation displays it as deprecated and states which
  endpoint replaces it

#### Scenario: Filter vocabulary is documented in depth

- **WHEN** a reader looks up how to query jobs by filters
- **THEN** the docs list every search facet param, the `<param>_mode=and` and
  `<param>_exclude` modifiers, the numeric (`salary_min`/`salary_max`/
  `experience_years_min`/`experience_years_max`) and boolean (`visa_sponsorship`)
  filters, full-text `q`, `sort`/`order`, and `semantic_ratio`, with at least one
  worked recipe

#### Scenario: The OpenAPI contract declares both experience bounds

- **WHEN** an endpoint in `web/static/openapi.yaml` declares the
  `experience_years_min` parameter
- **THEN** it also declares `experience_years_max`, described as the upper bound of
  the same range

## ADDED Requirements

### Requirement: Legacy per-endpoint documentation URLs redirect

The system SHALL redirect any request to a legacy per-endpoint documentation
path to `/docs/api` with an HTTP 301, rather than returning a 404.

#### Scenario: A bookmarked endpoint URL still resolves

- **WHEN** a client requests a legacy path of the shape
  `/docs/api/<group>/<endpoint>`
- **THEN** the server responds with an HTTP 301 redirect to `/docs/api`

### Requirement: The embedded reference matches the site's theme

The embedded API reference SHALL use the design system's color tokens for its
theme rather than a default preset theme, so it is visually consistent with
the rest of the site in both light and dark mode.

#### Scenario: Reference matches site theme

- **WHEN** a visitor views `/docs/api` in either light or dark mode
- **THEN** the embedded reference's colors are drawn from the same
  design-system tokens the rest of the site uses, not a Scalar built-in
  preset palette
