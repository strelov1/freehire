## MODIFIED Requirements

### Requirement: Company list is served without joining jobs

The system SHALL expose `GET /api/v1/companies` returning companies read from the
`companies` table. Each company's job count SHALL be read from the denormalized
`companies.job_count` column (open jobs only), not computed at query time, so the
read path performs no join to the `jobs` table. When no search query is present,
the list SHALL be ordered by `job_count` descending, then `name` ascending, so the
most active companies surface first.

The endpoint SHALL accept an optional `q` query parameter that searches companies
by their `name`, `slug`, and `tagline`. When `q` is non-empty, results SHALL be
ranked by **search relevance** — an exact name match first, then a prefix match,
then a contains match — with **typo tolerance**, and with `job_count` descending
used as the relevance tiebreaker so that among equally-relevant matches the most
active company surfaces first. In particular, a company whose name exactly equals
`q` SHALL rank ahead of companies that merely contain `q` in their name or slug,
regardless of the other companies' job counts. An absent or empty `q` SHALL return
the unfiltered list ordered by `job_count` descending then `name` ascending.

Company search SHALL be served by the Meilisearch companies index (see the
"Company search is served by a Meilisearch index with a Postgres fallback"
requirement). When the search index is disabled or unavailable, the endpoint SHALL
fall back to a case-insensitive substring match on the company `name`/`slug`
ordered by `job_count` descending then `name` ascending, so the endpoint always
returns a result.

The endpoint SHALL additionally accept repeatable facet query parameters —
`collections`, `regions`, `countries`, `domains`, `company_type`, `company_size`,
`remote_regions`, `yc_batch`, `yc_status`, `yc_stage`, and `yc_flags` — each
filtering against the company's corresponding denormalized array by **array
overlap**: a company matches a facet when its array shares at least one value with
the requested values (OR within a facet), and a company must match every provided
facet (AND across facets). The `remote_regions` facet filters the job-derived
remote-hiring regions (a subset of `regions`). The `yc_batch`, `yc_status`,
`yc_stage`, and `yc_flags` facets filter the curated YC-directory columns (see the
`yc-company-enrichment` capability); a non-YC company has them empty and matches
none. Facet filters SHALL compose with the `q` search. An absent facet
parameter SHALL not constrain the list.

The endpoint SHALL additionally accept the repeatable **scalar** facet parameters
`maturity` and `subindustries`, each filtering against a single-valued company
column (`companies.maturity` / `companies.subindustry`) by **membership**: a company
matches when its scalar value is among the requested values (OR within the facet),
and each ANDs with the others and with `q` exactly like the array facets. A company
whose column is `NULL` matches no value for that facet. `maturity` values are
`government`, `startup`, `scaleup`, `enterprise`; `subindustries` values are the
YC subindustry leaves served by `GET /api/v1/companies/subindustries`.

The hiring scope is preserved regardless of backend: only companies with
`job_count > 0` are eligible for the list and search results, exactly as the
Postgres path scopes today.

When any filter (`q` or a facet) is applied, the list `meta.total` SHALL report
the count of companies matching the full filter combination, so pagination over
the filtered results is correct.

#### Scenario: Listing companies most-active first

- **WHEN** a client requests `GET /api/v1/companies`
- **THEN** the response contains companies under `data` with list `meta`,
  ordered by `job_count` descending (ties broken by `name`), each carrying its
  denormalized `job_count`

#### Scenario: Searching companies ranks relevance first

- **WHEN** a client requests `GET /api/v1/companies?q=acme`
- **THEN** the response contains only companies matching `acme`, ranked by search
  relevance (exact name, then prefix, then contains) with `job_count` as the
  tiebreaker, and `meta.total` is the count of matching companies

#### Scenario: An exact-name match ranks first despite a low job count

- **WHEN** a company is named exactly `arb` (few open jobs) and other companies'
  names or slugs merely contain `arb` (many open jobs), and a client requests
  `GET /api/v1/companies?q=arb`
- **THEN** the company named exactly `arb` is the first result

#### Scenario: Search tolerates a typo

- **WHEN** a client requests `GET /api/v1/companies?q=arbnb` and a company named
  `Airbnb` exists
- **THEN** `Airbnb` is returned among the results

#### Scenario: Empty query returns the full list

- **WHEN** a client requests `GET /api/v1/companies?q=` (empty or absent)
- **THEN** the response is the unfiltered company list ordered by `job_count`
  descending then `name`, identical to omitting the parameter

#### Scenario: Filtering by a single facet

- **WHEN** a client requests `GET /api/v1/companies?regions=europe`
- **THEN** the response contains only companies whose `regions` array contains
  `europe`, and `meta.total` is the count of such companies

#### Scenario: Multiple values within one facet are OR-ed

- **WHEN** a client requests `GET /api/v1/companies?regions=europe&regions=asia`
- **THEN** the response contains companies whose `regions` overlap
  `{europe, asia}` (in Europe **or** Asia)

#### Scenario: Different facets are AND-ed and compose with search

- **WHEN** a client requests
  `GET /api/v1/companies?collections=yc&company_type=startup&q=lab`
- **THEN** the response contains only companies that are in the `yc` collection
  **and** have `startup` among their `company_types` **and** whose name matches
  `lab`

#### Scenario: Filtering by remote-hiring regions

- **WHEN** a client requests `GET /api/v1/companies?remote_regions=eu`
- **THEN** the response contains only companies whose `remote_regions` array
  contains `eu`, and `meta.total` is the count of such companies

#### Scenario: Filtering by YC facets

- **WHEN** a client requests `GET /api/v1/companies?yc_stage=Growth&yc_flags=top_company`
- **THEN** the response contains only companies whose `yc_stage` contains `Growth`
  **and** whose `yc_flags` contains `top_company`, and `meta.total` is the count of
  such companies

#### Scenario: Filtering by the scalar maturity facet

- **WHEN** a client requests `GET /api/v1/companies?maturity=startup&maturity=scaleup`
- **THEN** the response contains only companies whose `maturity` is `startup` **or**
  `scaleup`, excluding any company whose `maturity` is `NULL`, and `meta.total` is
  the count of such companies

#### Scenario: Filtering by the scalar subindustry facet

- **WHEN** a client requests
  `GET /api/v1/companies?subindustries=Payments&subindustries=Diagnostics`
- **THEN** the response contains only companies whose `subindustry` is `Payments`
  **or** `Diagnostics`, excluding any company whose `subindustry` is `NULL`, and
  `meta.total` is the count of such companies

## ADDED Requirements

### Requirement: Company search is served by a Meilisearch index with a Postgres fallback

The system SHALL maintain a Meilisearch **companies** index, separate from the
jobs index, holding one document per company with `job_count > 0`. Each document
SHALL carry the searchable text attributes `name`, `slug`, and `tagline`; the
sortable `job_count`; and, as filterable attributes, the denormalized facet arrays
(`collections`, `regions`, `countries`, `domains`, `company_types`,
`company_sizes`, `remote_regions`, `yc_batch`, `yc_status`, `yc_stage`, `yc_flags`)
and scalars (`maturity`, `subindustry`) used by the list endpoint. The index
SHALL be keyed by `slug`.

The index SHALL be built and refreshed by a scheduled full rebuild that reads the
`companies` table (scoped to `job_count > 0`) and atomically swaps the freshly
built index into place, so the search index is **eventually consistent** with the
`companies` table within the rebuild interval. The rebuild SHALL reuse the atomic
index-swap approach used by the jobs reindex and SHALL NOT run concurrently with
the jobs reindex on the same host.

When the search index is disabled (no Meilisearch configured) or a search request
against it fails, the list endpoint SHALL fall back to the Postgres substring path
without surfacing an error to the client, so company search gains no new failure
point relative to the pre-index behavior.

Building the companies index SHALL NOT modify the jobs index or its code path, so
the jobs search cannot regress.

#### Scenario: The companies index is rebuilt from Postgres

- **WHEN** the scheduled company reindex runs
- **THEN** a companies index is rebuilt from `companies` rows with `job_count > 0`
  and atomically swapped into place, and subsequent searches read the new index

#### Scenario: New company data appears after the next rebuild

- **WHEN** a company's `job_count` or facet arrays change (via the periodic
  recompute) between company reindex runs
- **THEN** the search results do not reflect the change until the next company
  reindex, which then includes it

#### Scenario: Endpoint falls back when the index is unavailable

- **WHEN** the Meilisearch companies index is unreachable and a client requests
  `GET /api/v1/companies?q=acme`
- **THEN** the endpoint serves the Postgres substring result (case-insensitive
  `name`/`slug` match ordered by `job_count` descending) and returns HTTP 200

#### Scenario: Endpoint falls back when search is disabled

- **WHEN** no Meilisearch is configured and a client requests
  `GET /api/v1/companies?q=acme`
- **THEN** the endpoint serves the Postgres substring result and returns HTTP 200

### Requirement: All company-search surfaces use the single companies endpoint

Every company search, typeahead, and ranked-list surface in the product SHALL be
served by `GET /api/v1/companies` and therefore by the Meilisearch companies index
(with its Postgres fallback). The system SHALL NOT introduce a separate company
search path that bypasses this endpoint. This covers the company catalog page, the
job-filter sidebar company typeahead, the referral company picker, and the global
header search's company results.

#### Scenario: Typeahead surfaces share the ranked search

- **WHEN** a user types a company name into the job-filter company typeahead, the
  referral company picker, or the global header search
- **THEN** the suggestions are produced by `GET /api/v1/companies?q=<typed>`, ranked
  by the same relevance-first ordering as the catalog search

#### Scenario: No bypassing company search path exists

- **WHEN** the codebase serves a company search or typeahead
- **THEN** it calls `GET /api/v1/companies` rather than a separate ad-hoc company
  search query
