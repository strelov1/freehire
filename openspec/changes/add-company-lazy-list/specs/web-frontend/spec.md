## ADDED Requirements

### Requirement: Company filter facet is a lazy, server-backed typeahead

The frontend job-search filter UI SHALL offer a "Company" facet that sources its
options from the companies endpoint (`GET /api/v1/companies?q=`), not from the
Meilisearch facet distribution. As the user types, the facet SHALL query the
endpoint (debounced) and present matching companies by their display `name`
alongside their global open-job count, ordered most-active first. With an empty
query the facet SHALL present the most popular companies (the endpoint's
count-ordered first page). Selecting a company SHALL apply the existing
`company_slug` search parameter, so URL state, exclusion, and the selected-value
chips behave identically to the other facets. The count shown for a company is
its global open-job count, not a count contextual to the other active filters.

#### Scenario: Typing finds a company outside the top results

- **WHEN** a user types "google" into the Company facet
- **THEN** the facet queries `GET /api/v1/companies?q=google` and lists matching
  companies by name with their job counts, even though "google" is not among the
  most popular companies shown for an empty query

#### Scenario: Empty query shows the most popular companies

- **WHEN** a user opens the Company facet without typing
- **THEN** the facet shows the most active companies first (highest job count),
  sourced from the endpoint's first page

#### Scenario: Selecting a company filters the job list

- **WHEN** a user selects a company in the facet
- **THEN** the search request carries that company's `company_slug` and the job
  results are limited to that company, and a removable chip shows the company's
  display name

#### Scenario: A pre-selected company from the URL is shown by name

- **WHEN** the page loads with `?company_slug=stripe` already set
- **THEN** the Company facet shows the selection as a removable chip with a
  human-readable label, without requiring the user to first search for it
