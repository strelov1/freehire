## MODIFIED Requirements

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
