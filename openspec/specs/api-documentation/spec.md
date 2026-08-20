# api-documentation

## Purpose

Provide a public, accurate reference for the freehire HTTP API — generated from a
single typed source so the rendered website page and the repo `docs/API.md`
cannot drift — covering every public endpoint, the response envelope, and the
job-search filter vocabulary, with a focus on querying jobs by filters.
## Requirements
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

`web/static/openapi.yaml` is the integration contract, so every endpoint that
declares `experience_years_min` SHALL also declare its companion
`experience_years_max`. The two SHALL be documented as a pair whose meaning is a
range over the posting's stated experience requirement, and the documentation SHALL
state that either bound excludes postings that state no requirement.

#### Scenario: Endpoint entry is complete

- **WHEN** the documentation lists an endpoint
- **THEN** it shows the HTTP method, the path, an authentication badge
  (none / cookie-or-key / cookie / moderator), its parameters, and a curl example

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

### Requirement: Published client identification convention

The published API documentation SHALL state a requested `User-Agent` format for
programmatic callers, and SHALL state that it is requested rather than required.

The recommended shape is `owner/project/version (+contact-url)` — the version
and contact URL optional, the owner and project name not. The documentation
SHALL say what identifying buys the caller: contact before a limit changes,
instead of a `429` as first notice.

The API SHALL NOT validate, require, or behave differently on the header. No
request is refused, delayed, budgeted differently, or logged as an error for
omitting it or for sending anything at all. Enforcement is deliberately
deferred: the convention is published to callers who predate it, and refusing
them for not following an instruction that did not exist when they integrated
would break working clients to enforce a courtesy.

This is recorded as a decision rather than left implicit, so that a later
reader finds a considered deferral instead of an unfinished feature. Should
identification ever gate anything, it SHALL be through a credential the server
issues, not a self-declared string a caller can set to any value.

#### Scenario: A caller sending no user agent is served normally

- **WHEN** a request arrives at any public endpoint with no `User-Agent` header,
  or with a generic HTTP-library default
- **THEN** it is served exactly as an identified caller's request would be, with
  the same rate-limit budget and the same response

#### Scenario: The convention is discoverable where integrators look

- **WHEN** an integrator reads the published schema, the llms.txt summary, or
  robots.txt
- **THEN** each states the requested format and that it is not enforced
