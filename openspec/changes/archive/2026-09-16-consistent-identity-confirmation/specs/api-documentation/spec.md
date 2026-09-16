## MODIFIED Requirements

### Requirement: Documented API coverage

The documentation SHALL cover the whole public API surface: the base URL, the
response envelope and pagination conventions, the public job reads
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
