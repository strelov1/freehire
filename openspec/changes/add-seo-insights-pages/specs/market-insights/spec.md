## ADDED Requirements

### Requirement: Category-scoped role demand

The `GET /api/v1/insights/roles` endpoint SHALL accept an optional `category`
parameter that restricts the ranked roles to that category's seniorities, so a
per-category roles view can be served in a single call. The parameter SHALL be
validated against the enrichment category vocabulary; an unknown value SHALL be a
`400`. When omitted, the endpoint behaves as before (all category × seniority pairs).

#### Scenario: Roles scoped to one category

- **WHEN** a client requests `GET /api/v1/insights/roles?category=backend`
- **THEN** the `data` array contains only `backend` roles (one row per seniority
  present), ranked by the requested sort, and `meta` echoes the `category`

#### Scenario: Unknown category rejected

- **WHEN** a client requests `GET /api/v1/insights/roles?category=not-a-category`
- **THEN** the response is `400` with an `{"error": ...}` body

### Requirement: All-seniority salary bands for a category

The system SHALL provide a way to read, in a single call, the salary bands for every
seniority within a category (each per currency and period), so a per-category salary
page does not need one request per seniority. Bands below the minimum sample size
SHALL remain suppressed as for the existing salary read.

#### Scenario: Category salary spans seniorities in one call

- **WHEN** a client requests the category salary read for `backend`
- **THEN** the response contains the salary bands for each seniority in `backend`
  that has a qualifying sample, grouped by seniority and currency, in one response
