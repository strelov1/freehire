## ADDED Requirements

### Requirement: Single typed source of truth for API docs

The system SHALL describe the public API as typed data in a single module
(`web/src/lib/docs/api-spec.ts`) from which both the rendered page and the
`docs/API.md` file are produced, so the two representations cannot drift.

#### Scenario: One source feeds both outputs

- **WHEN** an endpoint or parameter is added or edited in `api-spec.ts`
- **THEN** the rendered page reflects it on next build, and re-running the docs
  generator updates `docs/API.md` from the same data with no separate hand-edit

#### Scenario: Filter vocabulary derives from generated contracts

- **WHEN** the documented job-search filter table is built
- **THEN** its facet values come from `web/src/lib/generated/contracts.ts` and
  `web/src/lib/facets.ts` (the existing source of truth mirrored from Go
  `StringFacets`), not a hand-maintained duplicate list

### Requirement: Public API documentation page

The system SHALL serve a server-rendered documentation page at `/docs/api` that
is publicly accessible (no authentication) and documents the public HTTP API.

#### Scenario: Page is reachable and rendered server-side

- **WHEN** an unauthenticated visitor requests `/docs/api`
- **THEN** the server returns a fully rendered HTML page with the documentation
  content and a page title/meta suitable for SEO

#### Scenario: Page is discoverable from navigation

- **WHEN** a visitor views the top navigation
- **THEN** an "API" link points to `/docs/api`, and the CLI and API-keys pages
  cross-link to it as the full API reference

### Requirement: Documented API coverage

The documentation SHALL cover the whole public API surface: the base URL, the
response envelope and pagination conventions, the public job reads
(`/jobs`, `/jobs/search`, `/jobs/facets`, `/jobs/:slug`, `/jobs/:slug/similar`),
companies, authentication, API keys, per-user job interactions, submissions,
reports, and saved searches/subscriptions. Each endpoint SHALL state its method,
path, authentication requirement, parameters, and a copyable curl example.

#### Scenario: Endpoint entry is complete

- **WHEN** the documentation lists an endpoint
- **THEN** it shows the HTTP method, the path, an authentication badge
  (none / cookie-or-key / cookie / moderator), its parameters, and a curl example

#### Scenario: Filter vocabulary is documented in depth

- **WHEN** a reader looks up how to query jobs by filters
- **THEN** the docs list every search facet param, the `<param>_mode=and` and
  `<param>_exclude` modifiers, the numeric (`salary_min`/`salary_max`/
  `experience_years_min`) and boolean (`visa_sponsorship`) filters, full-text
  `q`, `sort`/`order`, and `semantic_ratio`, with at least one worked recipe

### Requirement: Generated Markdown reference

The system SHALL provide a generator script (run via a `gen:api-docs` npm
script) that writes `docs/API.md` from the typed spec data. The generated file
SHALL carry a header marking it as generated and not to be hand-edited.

#### Scenario: Generator produces the Markdown file

- **WHEN** `gen:api-docs` is run
- **THEN** `docs/API.md` is written from `api-spec.ts` and begins with a
  "generated — do not edit" header

#### Scenario: Regeneration is idempotent

- **WHEN** `gen:api-docs` is run twice with no source change in between
- **THEN** the second run produces a `docs/API.md` byte-identical to the first
