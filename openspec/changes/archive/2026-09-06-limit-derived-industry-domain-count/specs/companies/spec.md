## MODIFIED Requirements

### Requirement: Companies carry derived facet arrays aggregated from their open jobs

The system SHALL store, on each `companies` row, a set of denormalized facet
arrays derived from the company's **open** jobs (`closed_at IS NULL`):
`regions`, `countries`, `domains`, `company_types`, `company_sizes`,
`remote_regions`, and `industries_derived` (each a `TEXT[]`). Each array SHALL be
the **distinct union** of the corresponding value across the company's open jobs,
except `company_sizes` (an `employee_count`-authoritative hybrid) and
`industries_derived` (below), which are each described in their own terms:

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
- `industries_derived` is a **second-order** derivation, computed from the
  company's own `industries` and `domains` (not from `jobs` directly): it SHALL
  be empty whenever the company's curated `industries` is non-empty, or whenever
  `domains` holds more than two distinct values; otherwise it SHALL be the
  distinct set of industries that `domains` maps to through the curated
  domain→industry mapping. This is the materialized form of the precedence and
  domain-count threshold described in the industries-facet requirement below —
  computed once per recompute rather than evaluated per request.

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

#### Scenario: industries_derived is empty for a company with a curated industry

- **WHEN** the recompute runs for a company with a non-empty curated `industries`
  and `domains` `{fintech}`
- **THEN** that company's `industries_derived` is `{}`, regardless of `domains`

#### Scenario: industries_derived maps domains within the threshold

- **WHEN** the recompute runs for a company with an empty curated `industries` and
  `domains` `{devtools, fintech}` (two distinct values)
- **THEN** that company's `industries_derived` is `{developer-tools, fintech}`

#### Scenario: industries_derived is empty above the domain-count threshold

- **WHEN** the recompute runs for a company with an empty curated `industries` and
  `domains` `{fintech, gamedev, healthcare}` (three distinct values)
- **THEN** that company's `industries_derived` is `{}`

### Requirement: The company industry facet resolves through two sources

The `industries` facet on `GET /api/v1/companies` SHALL match a company through two
**ordered** sources: the curated `companies.industries` array, and — only where that
array is empty **and** the company carries at most two distinct job-derived domains —
the job-derived `companies.domains` array translated through a curated
domain→industry mapping. A company an importer has classified SHALL be answered from
that classification alone, and its domains SHALL NOT be consulted. A company with no
curated industry but three or more distinct domains SHALL also NOT be matched through
its domains. The facet otherwise behaves exactly like the other array facets (OR
within the facet, AND across facets, composing with `q`).

The sources are ordered rather than equal because they are not equally about the
company. `companies.domains` is the union of the enrichment domain over every open
job the company holds, so for a company with many postings it drifts from what the
company is toward the range of work it advertises: a large ride-hailing company
accumulates `gamedev`, `edtech` and `govtech` from individual openings. Consulting
that union for a company that has already been classified adds no reach and asserts
industries the company is not in. The same drift correlates with domain count even
for a company with no curated industry at all: a focused company's postings carry one
or two domains, and three or more marks the union as describing hiring range rather
than business — `freenow` (no curated industry, seven domains) is reachable under
`fintech`, `gamedev`, `healthcare`, `mobility`, `saas`, `travel` today, none of them
its actual business.

The mapping SHALL be dict-only, in keeping with every other dictionary in the
system: a domain value that names no curated industry honestly — including `other`,
and any value absent from `vocab.DomainValues` — SHALL map to nothing and SHALL
contribute no industry. The mapping SHALL never invent a canonical value: every
industry it produces must already exist in the curated vocabulary.

Both query backends SHALL implement this identically, precedence and the domain-count
threshold included. `GET /api/v1/companies` is served by the Meilisearch companies
index or by Postgres depending on the request, and the rendered list and its
`meta.total` may be produced by different paths within one page; a company matching
on one backend and not the other would make a page contradict its own count.

The `domains` query parameter SHALL keep filtering on the domain column directly,
unchanged. Widening `industries` removes a duplicate *control*, not a contract.

#### Scenario: A company matches on its curated industry

- **WHEN** a client requests `GET /api/v1/companies?industries=fintech` and a company
  has `industries` containing `fintech`
- **THEN** that company is in the response, regardless of its `domains`

#### Scenario: A curated company is not matched through its domains

- **WHEN** a company has a non-empty `industries` that does not contain the requested
  value, and a `domains` array that maps to it
- **THEN** that company is absent from the response, on either backend

#### Scenario: A company matches on its job-derived domain alone

- **WHEN** a client requests `GET /api/v1/companies?industries=developer-tools` and a
  company has an empty `industries` and at most two distinct `domains`, one of them
  `devtools`
- **THEN** that company is in the response

#### Scenario: A company with no curated industry and three or more domains is not matched through its domains

- **WHEN** a company has an empty `industries` and three or more distinct `domains`,
  one of them mapping to the requested industry
- **THEN** that company is absent from every `industries`-filtered response, even
  though the same domain value would match on a company with one or two domains

#### Scenario: The domain-count threshold is inclusive at two

- **WHEN** a company has an empty `industries` and exactly two distinct `domains`,
  one of them mapping to the requested industry
- **THEN** that company is in the response

#### Scenario: An unmapped domain yields no industry

- **WHEN** a company has an empty `industries` and its `domains` holds only values the
  mapping does not cover — `other`, or a value absent from `vocab.DomainValues` such
  as `saas`
- **THEN** that company matches no `industries` value and is absent from every
  `industries`-filtered response

#### Scenario: Both backends agree on the matched set

- **WHEN** the same `industries` filter is served once by the Meilisearch path and
  once by the Postgres path
- **THEN** both return the same set of companies, so a page's list and its
  `meta.total` cannot disagree

#### Scenario: The domain parameter is unaffected

- **WHEN** a client requests `GET /api/v1/companies?domains=devtools`
- **THEN** the response contains exactly the companies whose `domains` array contains
  `devtools`, as before this change
