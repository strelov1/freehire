# api-documentation

## Purpose

Provide a public, accurate reference for the freehire HTTP API — generated from a
single typed source so the rendered website pages and the repo `docs/API.md`/
`docs/API.internal.md` cannot drift — covering every endpoint, the response
envelope, and the job-search filter vocabulary, with a focus on querying jobs by
filters.

The API has two audiences with different reach: endpoints a caller outside the
browser can reach (no auth, or a personal API key) versus endpoints that require
the browser's own session cookie (including moderator- and browser-extension-only
ones) and so are unreachable by any external client or agent regardless of
credential. The documentation is split along exactly that line into an external
reference (`/docs/api`) and an internal one (`/docs/api/internal`), generated
from the same source so the two can never disagree about which endpoint belongs
where.

## Requirements

### Requirement: Single typed source of truth for API docs

The system SHALL describe the whole API as typed data in a single module
(`web/src/lib/docs/api-spec.ts`) from which both rendered pages, both generated
OpenAPI documents, and both `docs/API.md`/`docs/API.internal.md` files are
produced, so the six representations cannot drift. Each endpoint's declared
`auth` level SHALL be the sole input deciding which of the two documents it
appears in: `none` and `cookie-or-key` (reachable from outside the browser) go
to the external document; `cookie`, `moderator`, and `extension` (reachable only
with the browser's own session cookie) go to the internal one. A group left
with no endpoints for a given audience SHALL NOT appear in that audience's
document.

#### Scenario: One source feeds all outputs

- **WHEN** an endpoint or parameter is added or edited in `api-spec.ts`
- **THEN** both generated OpenAPI documents reflect it on next generation, both
  rendered pages (which render those generated OpenAPI documents) reflect it
  once regenerated, and re-running the docs generators updates both
  `docs/API.md` and `docs/API.internal.md` from the same data — with no
  separate hand-edit of any of the six

#### Scenario: An endpoint's auth level decides its document

- **WHEN** an endpoint is added to `api-spec.ts` with `auth: 'none'` or
  `auth: 'cookie-or-key'`
- **THEN** it appears only in the external document (`/docs/api`,
  `docs/API.md`) and never in the internal one
- **WHEN** an endpoint is added with `auth: 'cookie'`, `auth: 'moderator'`, or
  `auth: 'extension'`
- **THEN** it appears only in the internal document (`/docs/api/internal`,
  `docs/API.internal.md`) and never in the external one

#### Scenario: Filter vocabulary derives from generated contracts

- **WHEN** the documented job-search filter table is built
- **THEN** its facet values come from `web/src/lib/generated/contracts.ts` and
  `web/src/lib/facets.ts` (the existing source of truth mirrored from Go
  `StringFacets`), not a hand-maintained duplicate list

### Requirement: Public API documentation page

The system SHALL serve a server-rendered documentation page at `/docs/api` that
is publicly accessible (no authentication) and documents every endpoint
reachable from outside the browser (`auth: 'none'` and `auth: 'cookie-or-key'`).
It SHALL be indexable and linked from navigation.

#### Scenario: Page is reachable and rendered server-side

- **WHEN** an unauthenticated visitor requests `/docs/api`
- **THEN** the server returns a fully rendered HTML page with the documentation
  content and a page title/meta suitable for SEO

#### Scenario: Page is discoverable from navigation

- **WHEN** a visitor views the top navigation
- **THEN** an "API" link points to `/docs/api`, and the CLI and API-keys pages
  cross-link to it as the full API reference

### Requirement: Internal (session-only) API documentation page

The system SHALL serve a server-rendered documentation page at
`/docs/api/internal` that documents every endpoint reachable only with the
browser's own session cookie (`auth: 'cookie'`, `auth: 'moderator'`, and
`auth: 'extension'`) — none of which a personal API key or an anonymous request
can call. The page SHALL be reachable by direct link but SHALL NOT be indexed
by search engines and SHALL NOT be linked from site navigation, the footer, or
any agent-facing landing page, since nothing documented there is callable by an
external client or agent.

#### Scenario: Page is reachable but not indexed

- **WHEN** a visitor requests `/docs/api/internal`
- **THEN** the server returns a fully rendered HTML page with the documentation
  content, carrying a `noindex` robots directive

#### Scenario: Page is not promoted to an external audience

- **WHEN** a visitor views the top navigation, the footer, or an agent-facing
  landing page (CLI, ChatGPT Actions, "for agents")
- **THEN** none of them link to `/docs/api/internal`

#### Scenario: Each document cross-links to the other

- **WHEN** a reader views either `/docs/api` or `/docs/api/internal`
- **THEN** its overview states which endpoints are documented there and links
  to the other document for the rest

### Requirement: Documented API coverage

Between the two documents, the whole API surface SHALL be covered: the base
URL, the response envelope and pagination conventions, the public job reads
(`/jobs`, `/jobs/search`, `/jobs/facets`, `/jobs/:slug`, `/jobs/:slug/similar`),
companies, authentication, per-user job interactions, submissions,
reports, and saved searches/subscriptions. Each endpoint SHALL state its method,
path, authentication requirement, parameters, and a copyable curl example,
rendered via the embedded OpenAPI reference.

The reference SHALL NOT document an endpoint that no API client can call. An endpoint that
requires a session cookie **and** a proof of recent credential control is unreachable from a
script by design: every scripted attempt answers `428` regardless of what the reader does, so
documenting it — and in particular offering a copyable curl for it — describes a request that
cannot succeed. The API-key management endpoints (`POST`, `GET` and `DELETE` under
`/me/api-keys`) are such endpoints and SHALL be omitted from the endpoint reference.

Where such an endpoint is omitted, the documentation SHALL say so in its "what is not here"
section rather than leave the reader to notice the absence, SHALL state why calling it directly
is not possible, and SHALL name the product surface that performs the action, with a link to it.

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

#### Scenario: Key management is not documented as an endpoint

- **WHEN** a reader browses the endpoint reference or the generated Markdown
- **THEN** neither lists `POST /me/api-keys`, `GET /me/api-keys`, or
  `DELETE /me/api-keys/{id}`, and neither offers a curl example for them

#### Scenario: The omission is explained and redirected

- **WHEN** a reader looks for how to obtain an API key
- **THEN** the "what is not here" section states that key management cannot be called from a
  script, and links to the account surface where keys are created and revoked

### Requirement: Generated Markdown reference

The system SHALL provide a generator script (run via a `gen:api-docs` npm
script) that writes both `docs/API.md` (external) and `docs/API.internal.md`
(internal) from the typed spec data. Each generated file SHALL carry a header
marking it as generated and not to be hand-edited.

#### Scenario: Generator produces both Markdown files

- **WHEN** `gen:api-docs` is run
- **THEN** `docs/API.md` and `docs/API.internal.md` are each written from
  `api-spec.ts`, each split per the endpoint `auth` rule above, and each
  begins with a "generated — do not edit" header

#### Scenario: Regeneration is idempotent

- **WHEN** `gen:api-docs` is run twice with no source change in between
- **THEN** the second run produces a `docs/API.md` and a `docs/API.internal.md`
  each byte-identical to their first run's output

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

### Requirement: Legacy per-endpoint documentation URLs redirect

The system SHALL redirect any request to a legacy per-endpoint documentation
path to `/docs/api` with an HTTP 301, rather than returning a 404.

#### Scenario: A bookmarked endpoint URL still resolves

- **WHEN** a client requests a legacy path of the shape
  `/docs/api/<group>/<endpoint>`
- **THEN** the server responds with an HTTP 301 redirect to `/docs/api`

### Requirement: The embedded reference matches the site's theme

Both embedded API references SHALL use the design system's color tokens for
their theme rather than a default preset theme, so each is visually consistent
with the rest of the site in both light and dark mode.

#### Scenario: Reference matches site theme

- **WHEN** a visitor views `/docs/api` or `/docs/api/internal` in either light
  or dark mode
- **THEN** the embedded reference's colors are drawn from the same
  design-system tokens the rest of the site uses, not a Scalar built-in
  preset palette
