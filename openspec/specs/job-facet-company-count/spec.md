# job-facet-company-count Specification

## Purpose
TBD - created by archiving change talent-agent-ai-search. Update Purpose after archive.
## Requirements
### Requirement: The facets endpoint reports distinct companies on request

`GET /api/v1/jobs/facets` SHALL accept `company_slug` in its `facets` parameter. When requested, the response SHALL include `companies_total` — the exact number of distinct `company_slug` values among the open postings matching the filter — and `top_companies` — at most 50 entries `{slug, name, count}` in descending job count (ties by slug), named from the `companies` table or by the slug when the company has no row yet. These are separate fields rather than a `company_slug` entry in `facets`, because a JSON object can carry neither the order nor the names. `company_slug` SHALL stay out of the default facet set, and SHALL be refused under `disjunctive` and for an `OR` query (which runs as a federated search and would count a company matched by two alternatives twice). The count SHALL be exact regardless of the facet value cap.

#### Scenario: Exact count above the facet cap

- **WHEN** a filter matches postings from 1,540 distinct companies and the facet value cap is 1,200
- **THEN** `companies_total` is 1,540

#### Scenario: Top companies are ordered by job count

- **WHEN** `facets=company_slug` is requested
- **THEN** `top_companies` holds at most 50 entries in descending job count, each with a display name

#### Scenario: Not requested, not computed

- **WHEN** the request does not name `company_slug` in `facets`
- **THEN** the response carries no `companies_total` or `top_companies` and no extra search runs

#### Scenario: Disjunctive requests refuse it

- **WHEN** `disjunctive=1&facets=company_slug` is requested
- **THEN** the endpoint responds 400

#### Scenario: An OR query refuses it

- **WHEN** `q=go OR rust&facets=company_slug` is requested
- **THEN** the endpoint responds 400

