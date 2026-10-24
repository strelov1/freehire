## ADDED Requirements

### Requirement: The facets endpoint reports distinct companies on request

`GET /api/v1/jobs/facets` SHALL accept `company_slug` in its `facets` parameter. When requested, the response SHALL include `companies_total` — the exact number of distinct `company_slug` values among the open postings matching the filter — and a `company_slug` distribution trimmed to the top 50 by job count, each value carrying the company's display name. `company_slug` SHALL stay out of the default facet set, and SHALL be refused under `disjunctive`. The count SHALL be exact regardless of the facet value cap.

#### Scenario: Exact count above the facet cap

- **WHEN** a filter matches postings from 1,540 distinct companies and the facet value cap is 1,200
- **THEN** `companies_total` is 1,540

#### Scenario: Top companies are ordered by job count

- **WHEN** `facets=company_slug` is requested
- **THEN** the `company_slug` distribution holds at most 50 entries in descending job count, each with a display name

#### Scenario: Not requested, not computed

- **WHEN** the request does not name `company_slug` in `facets`
- **THEN** the response carries no `companies_total` and no extra search runs

#### Scenario: Disjunctive requests refuse it

- **WHEN** `disjunctive=1&facets=company_slug` is requested
- **THEN** the endpoint responds 400
