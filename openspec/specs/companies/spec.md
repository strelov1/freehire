# companies

## Purpose

Store companies as a first-class, slug-keyed entity linked from jobs, so the API
can serve a company catalog and a company-detail view (company + its jobs)
without joining the `jobs` table on the hot read paths.
## Requirements
### Requirement: Companies are stored as a slug-keyed entity

The system SHALL store companies in a `companies` table identified by a natural
`slug` key derived by normalizing the company name. The table SHALL NOT use a
surrogate id. Each company SHALL have a display `name`.

A company row is normally created from a job's company name. A company MAY also
exist as a **reference row** created by the company info backfill without any
job referencing it; such a row SHALL be marked `is_reference = true`. Any orphan
cleanup that deletes companies no job references SHALL preserve reference rows.
When a job is later ingested whose normalized name matches a reference row's
`slug`, the existing row SHALL be reused (not duplicated), gaining jobs while
keeping its company-info data.

#### Scenario: Company is created from a job's company name

- **WHEN** a job is ingested with a non-empty company name that has no matching
  company row
- **THEN** the system inserts a `companies` row whose `slug` is the normalized
  name and whose `name` is the display name

#### Scenario: Existing company is reused, not duplicated

- **WHEN** a job is ingested whose normalized company name matches an existing
  `companies.slug`
- **THEN** no duplicate company row is created and the existing row is reused

#### Scenario: Reference company survives orphan cleanup

- **WHEN** the orphan cleanup runs and a company row has no job referencing it
  but is marked `is_reference = true`
- **THEN** the row is not deleted

#### Scenario: A job adopts a reference company

- **WHEN** a job is ingested whose normalized company name matches the `slug` of
  a reference row
- **THEN** the existing row is reused, its job count reflects the new job, and
  its company-info fields are unchanged

### Requirement: Jobs link to a company via a denormalized key

The system SHALL store `company_slug` on each job as the normalized link key,
kept alongside the existing `company` display name. Jobs with an empty company
name SHALL have an empty `company_slug` and SHALL NOT create a company.

#### Scenario: Job carries both display name and link key

- **WHEN** a job with company name "Yandex LLC" is ingested
- **THEN** the job's `company` is the display name and its `company_slug` is the
  normalized key, and a matching `companies` row exists with that `slug`

#### Scenario: Job with no company

- **WHEN** a job is ingested with an empty company name
- **THEN** the job is stored with an empty `company_slug` and no company row is
  created

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

#### Scenario: Searching companies by name

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

### Requirement: Company job counts are denormalized and periodically recomputed

The system SHALL store each company's count of open jobs (`closed_at IS NULL`) in
a denormalized `companies.job_count` column, and its derived facet arrays
(`regions`, `countries`, `domains`, `company_types`, `company_sizes`,
`remote_regions`) in denormalized columns. Both SHALL be maintained by the same
periodic recompute (a scheduled worker), not by a synchronous write on the job
ingest/close paths, so they are eventually consistent with the `jobs` table within
the recompute interval. A company with no open jobs SHALL have `job_count = 0` and
empty facet arrays. `remote_regions` SHALL be maintained as the distinct union of
`regions` over the company's open jobs with `work_mode = 'remote'`, so it is a
subset of the `regions` array; a company with no open remote job has an empty
`remote_regions`.

The same recompute SHALL additionally maintain one **deterministic, single-valued**
classification column, `companies.maturity`, computed from signals already stored
(`organization_type`, `yc_status`, `employee_count`, `year_founded`, and whether
the company's open jobs come from an exclusively-government `source`). The
derivation SHALL be a pure rule (no LLM), applied in precedence order: `maturity`
is `government` when the company is government-sourced or `organization_type` is
`Government`, else `startup` when it is a YC company or is small and recently
founded, else `enterprise` when its employee count is large, else `scaleup` for
mid-size, else `NULL` (unknown). Where signals are silent, the value SHALL be
`NULL` — an honest abstain, never a fabricated label. This column SHALL be
maintained under the same `IS DISTINCT FROM` change-guard as the other facets, so
an unchanged company is not rewritten.

#### Scenario: Recompute reflects only open jobs

- **WHEN** the recompute runs and a company has 3 open jobs and 2 closed jobs
  (`closed_at` set)
- **THEN** that company's `job_count` is set to 3 and its facet arrays reflect
  only the 3 open jobs

#### Scenario: Recompute zeroes a company whose jobs all closed

- **WHEN** every job of a company has been closed since the last recompute and the
  recompute runs again
- **THEN** that company's `job_count` is set to 0 and its facet arrays are emptied

#### Scenario: Counts are eventually consistent

- **WHEN** a new job is ingested for a company between recompute runs
- **THEN** the company's `job_count` and facet arrays do not change until the next
  recompute, which then includes the new job

#### Scenario: Recompute rewrites nothing when already current

- **WHEN** the recompute runs and a company's `job_count` and every facet array
  already equal the freshly computed values
- **THEN** that company's row is not rewritten (the recompute reports it as
  unchanged)

#### Scenario: remote_regions is derived from open remote jobs only

- **WHEN** the recompute runs for a company whose open jobs are one `remote` job in
  `eu` and one `onsite` job in `north_america`
- **THEN** that company's `remote_regions` is `{eu}` (the onsite job's region is
  excluded) while its `regions` is `{eu, north_america}`

#### Scenario: maturity is derived deterministically

- **WHEN** the recompute runs for a YC company with `employee_count = 20`
- **THEN** its `maturity` is `startup`

#### Scenario: Unknown maturity abstains to NULL

- **WHEN** the recompute runs for a company with no government source, no YC
  status, and no `employee_count`
- **THEN** its `maturity` is `NULL` (unknown), not a guessed label

### Requirement: Company detail returns the company with its jobs

The system SHALL expose `GET /api/v1/companies/:slug` returning the company and
its **open** jobs (`closed_at IS NULL`). The company SHALL be read from
`companies` and its jobs from a single-table filter on `jobs.company_slug` —
without a SQL join between the two tables.

#### Scenario: Existing company

- **WHEN** a client requests `GET /api/v1/companies/:slug` for an existing slug
- **THEN** the response contains the company and its open jobs ordered like the
  main jobs listing

#### Scenario: Unknown company

- **WHEN** a client requests `GET /api/v1/companies/:slug` for a slug with no
  company row
- **THEN** the system responds with HTTP 404

#### Scenario: Closed job leaves the company page

- **WHEN** a company's job is closed
- **THEN** the company detail no longer lists it

### Requirement: Companies carry derived facet arrays aggregated from their open jobs

The system SHALL store, on each `companies` row, a set of denormalized facet
arrays derived from the company's **open** jobs (`closed_at IS NULL`):
`regions`, `countries`, `domains`, `company_types`, `company_sizes`, and
`remote_regions` (each a `TEXT[]`). Each array SHALL be the **distinct union** of
the corresponding value across the company's open jobs, except `company_sizes`
which is an `employee_count`-authoritative hybrid (below):

- `regions` and `countries` from the top-level `jobs.regions` / `jobs.countries`
  columns.
- `remote_regions` from `jobs.regions` but restricted to jobs with
  `work_mode = 'remote'` — the regions the company hires remotely in, always a
  subset of `regions`.
- `domains`, `company_types` from the job's `enrichment` payload (`domains`
  array, `company_type` scalar); an unenriched or value-less job contributes
  nothing, so these arrays are sparse until jobs are enriched.
- `company_sizes` is a **dict-then-LLM hybrid**: when the company has a known
  `companies.employee_count`, the array SHALL be the single authoritative size
  bucket derived from it (bucketed into the `company_size` vocabulary
  `1-10`/`11-50`/`51-200`/`201-500`/`501-1000`/`1000+`); when `employee_count` is
  absent, it SHALL fall back to the distinct union of `enrichment.company_size`
  over the company's open jobs. The `employee_count` value is a recorded company
  fact and is more accurate than the LLM's per-posting guess, so it wins when
  present.

A company with no open jobs SHALL have every facet array empty (`'{}'`), except
that `company_sizes` still reflects the company's `employee_count` bucket when one
is stored (it is a company fact, independent of open jobs). The arrays SHALL be
maintained by the same periodic recompute that maintains `job_count` (see the
recompute requirement), not by a synchronous write on the ingest/close paths, so
they are eventually consistent with `jobs`.

#### Scenario: Region and country unions are derived from open jobs

- **WHEN** the recompute runs for a company whose open jobs have regions
  `{europe}`, `{europe, asia}` and countries `{de}`, `{de, sg}`
- **THEN** that company's `regions` is `{asia, europe}` and `countries` is
  `{de, sg}` (distinct union, closed jobs excluded)

#### Scenario: remote_regions unions only the remote jobs' regions

- **WHEN** the recompute runs for a company whose open jobs are a `remote` job in
  `{eu}`, a `remote` job in `{apac}`, and an `onsite` job in `{north_america}`
- **THEN** that company's `remote_regions` is `{apac, eu}` and its `regions` is
  `{apac, eu, north_america}`

#### Scenario: company_sizes uses the employee_count bucket when known

- **WHEN** the recompute runs for a company with `employee_count = 320` whose open
  jobs carry `enrichment.company_size` values `11-50` and `51-200`
- **THEN** that company's `company_sizes` is `{201-500}` (the authoritative
  headcount bucket), not the LLM union

#### Scenario: company_sizes falls back to the enrichment union without employee_count

- **WHEN** the recompute runs for a company with no `employee_count` whose open,
  enriched jobs carry `enrichment.company_size` `11-50`
- **THEN** that company's `company_sizes` is `{11-50}` (the enrichment union)

#### Scenario: Other enrichment facets are derived from the enrichment payload

- **WHEN** the recompute runs for a company whose open, enriched jobs carry
  `enrichment.domains` `{fintech}` and `{fintech, ecommerce}` and
  `enrichment.company_type` `startup` and `product`
- **THEN** that company's `domains` is `{ecommerce, fintech}` and `company_types`
  is `{product, startup}`

#### Scenario: Unenriched jobs contribute no enrichment facets

- **WHEN** a company's only open job has never been enriched (empty `enrichment`)
  and the company has no `employee_count`
- **THEN** that company's `domains`, `company_types`, and `company_sizes` are all
  empty, while `regions`/`countries` still reflect the job's geography columns

#### Scenario: Closing all jobs empties the job-derived facet arrays

- **WHEN** every open job of a company (with no `employee_count`) is closed and the
  recompute runs again
- **THEN** that company's facet arrays are all set to empty (`'{}'`)

### Requirement: Companies carry a clean YC subindustry classification

The system SHALL store, on each `companies` row, a nullable scalar `subindustry` (`TEXT`)
holding the leaf of the company's YC subindustry path (e.g. `Industrials -> Manufacturing and
Robotics` → `Manufacturing and Robotics`). It SHALL be populated by `cmd/import-yc` from the
directory entry's `subindustry` field and SHALL be distinct from the existing
`companies.industries` array, which continues to hold the flattened, tag-inclusive display bag
unchanged. A company with no YC subindustry (a non-YC company, or a YC entry without one) SHALL
have `subindustry = NULL`. The value SHALL be a clean, human-readable taxonomy leaf, not a free
tag, so it is safe to offer as a filter option.

#### Scenario: Import stores the subindustry leaf

- **WHEN** `cmd/import-yc` imports a directory entry with `subindustry`
  `"Industrials -> Manufacturing and Robotics"`
- **THEN** that company's `companies.subindustry` is `"Manufacturing and Robotics"` and its
  `companies.industries` display array is unchanged

#### Scenario: A non-YC company has no subindustry

- **WHEN** a company has no matching YC directory entry
- **THEN** its `companies.subindustry` is `NULL`

### Requirement: The subindustry facet vocabulary is served dynamically with counts

The system SHALL expose `GET /api/v1/companies/subindustries` returning, under `data`, the
distinct non-`NULL` `companies.subindustry` values each with the count of companies carrying it,
ordered by count descending then value ascending. This serves the searchable option list for the
subindustry facet; the counts are unconditional (they do not reflect other active filters — a
deliberate simplification versus the Meilisearch job facets). Each item SHALL carry `value` and
`count`.

#### Scenario: Listing available subindustries with counts

- **WHEN** a client requests `GET /api/v1/companies/subindustries`
- **THEN** the response lists every distinct non-`NULL` subindustry with its company count,
  most common first

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

