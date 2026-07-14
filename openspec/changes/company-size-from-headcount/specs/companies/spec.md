# companies

## MODIFIED Requirements

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
