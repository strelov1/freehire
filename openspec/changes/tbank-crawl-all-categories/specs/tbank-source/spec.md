## ADDED Requirements

### Requirement: T-Bank careers-API crawl across all categories

The system SHALL provide a `tbank` source adapter that crawls T-Bank's public
careers API (`www.tbank.ru/pfpjobs/papi`) into the catalogue. It is a **boardless
single-company** adapter: config entries carry no board, the host is fixed, and it
is excluded from the source facet. The crawl is keyless and two-phase — a
`getVacancies` list request enumerates postings, and each posting's description is
fetched from a per-vacancy `getVacancyDescription` detail request.

The list request MUST filter on the full set of T-Bank's top-level vacancy
categories (`tcareer_it`, `tcareer_back_office`, `tcareer_work_with_clients`). An
empty category filter is not equivalent to "all categories" — the `publisher`
source defaults it to `tcareer_work_with_clients` alone, which excludes every IT
and back-office vacancy. The categories are a curated constant slice; adding a
future category is a one-line change.

#### Scenario: List request carries the full category set

- **WHEN** the adapter issues a `getVacancies` list request
- **THEN** the request body's `filters.category` contains every top-level category
  (`tcareer_it`, `tcareer_back_office`, `tcareer_work_with_clients`), so IT and
  back-office vacancies are enumerated alongside client roles

#### Scenario: Offset pagination drains every category in one loop

- **WHEN** the server reports more results via `nextPagination.publisher.offset`
  with `isFinished=false`
- **THEN** the adapter follows the offset until `isFinished` (or the offset stops
  advancing), accumulating vacancies across all filtered categories in a single loop

#### Scenario: Each vacancy maps to a Job with a stable identity

- **WHEN** the adapter fetches a vacancy's detail
- **THEN** it returns a `Job` whose `ExternalID` is the vacancy `urlSlug`, whose
  `URL` is the five-segment careers route ending in that `urlSlug`, and whose
  description is the assembled, sanitized HTML of the detail blocks — so re-crawling
  dedups to the same catalogue row

#### Scenario: A failed detail request skips only that vacancy

- **WHEN** one vacancy's `getVacancyDescription` request fails
- **THEN** the adapter omits just that vacancy and still returns the rest of the crawl
