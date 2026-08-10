## MODIFIED Requirements

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

#### Scenario: Multi-value params show the compact form

- **WHEN** a reader looks up how to pass multiple values for a facet param
- **THEN** the docs show the comma-separated form (e.g. `skills=go,react`) as
  the primary example, with a note that repeating the key
  (`skills=go&skills=react`) is also accepted
