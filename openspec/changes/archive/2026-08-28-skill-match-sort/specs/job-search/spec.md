## MODIFIED Requirements

### Requirement: Searchable jobs index

The system SHALL maintain a Meilisearch index of jobs with one document per job,
keyed by the job's internal `id`. Each document SHALL carry the fields needed to
both match and render a result without a follow-up database read: the searchable
text (title, company, description, location), the filterable facets, the
sortable fields, and the display fields returned to clients.

The index SHALL declare:
- **searchable attributes**: title, company, description, location.
- **filterable attributes**: source, company_slug, work_mode, employment_type,
  seniority, category, domains, regions, countries, company_type, company_size,
  visa_sponsorship, salary_currency, salary_period, skills, salary_min,
  salary_max, experience_years_min, and `posted_ts`. The raw `remote` flag SHALL
  NOT be a filterable attribute (work_mode subsumes it).
- **sortable attributes**: posted_at, created_at, enrichment.salary_min,
  enrichment.salary_max. The endpoint's `sort` values are the bare names — a
  request says `sort=salary_min`, which the handler maps to the nested attribute.
  The two salary facets live under `enrichment`, the two dates do not, and this
  list names the ATTRIBUTES rather than the aliases. (`created_at` was already
  accepted and already declared in code; this list had drifted.)
- **one embedder**, named for the skill vector it carries, declared as
  `userProvided` at the width `skill-match-sort` assigns. Being `userProvided`
  means Meilisearch SHALL NOT call any model: the vectors are supplied by the
  indexers. Binary quantization SHALL be off — the vectors are sparse (a handful of
  non-zeros out of hundreds of dimensions), and quantizing them was measured to drop
  recall@20 from 95% to 10%.

The declared embedder width SHALL NOT change without a full index rebuild, and
until such a rebuild completes the index rejects documents carrying the new width.

Each document SHALL carry a derived numeric `posted_ts` field: the unix-seconds
value of the job's **effective** posting date — the source's `posted_at` when
present and not in the future, otherwise the ingest time (`created_at`) — the
same value, in epoch form, that the document's display `posted_at` reflects.
`posted_ts` is an index-only field: it SHALL be filterable but SHALL NOT appear
in the public job wire shape returned by the job read endpoints. Because
`posted_ts` is derived at index time, no Postgres column or backfill is
required; a reindex SHALL populate it on existing jobs.

Geography and work mode are filtered through the document's **top-level**
`regions`, `countries`, and `work_mode` fields — the resolved union/precedence of
the location-derived columns and the enrichment-derived values — not through the
`enrichment.*` dot paths. There SHALL be no separate
`enrichment.regions`/`enrichment.countries`/`enrichment.work_mode` facet on the
document.

Facets derived from a job's `enrichment` JSONB SHALL be absent (or empty) on the
document when the job is not yet enriched; an unenriched job SHALL still be
indexed and findable by its text fields, and SHALL still carry any geography
parsed from its location.

A job with no skill vector SHALL still be indexed and fully searchable: the vector's
absence affects the match ordering alone, never that job's presence in the index or
its matching by text or any facet.

#### Scenario: A job is represented as one searchable document

- **WHEN** a job with title "Senior Go Developer", company "Acme", and a
  description is indexed
- **THEN** the `jobs` index holds one document keyed by that job's `id` whose
  searchable text includes the title, company, and description

#### Scenario: Unenriched job is still indexed with its parsed geography

- **WHEN** a job with no enrichment but location `Remote - USA` is indexed
- **THEN** the document is present and matchable by its text, with its
  enrichment-derived facets absent or empty and its top-level `regions`/
  `countries` carrying the parsed geography

#### Scenario: Geography is filterable via the top-level regions facet

- **WHEN** a job whose unioned geography includes `eu` is indexed
- **THEN** it is returned by a filter on `regions = "eu"`

#### Scenario: Document carries the effective posting date as an epoch

- **WHEN** a job whose effective posting date is a given instant is indexed
- **THEN** its document carries `posted_ts` equal to that instant in unix
  seconds, and a job with a null or future `posted_at` carries the `created_at`
  instant instead — matching its display `posted_at`

#### Scenario: posted_ts is filterable but not in the public job shape

- **WHEN** a job document is indexed and the same job is read through a public
  job endpoint
- **THEN** the document is filterable by a `posted_ts` numeric range, while the
  public job wire shape does not include a `posted_ts` field

#### Scenario: The index declares a model-free embedder

- **WHEN** the index settings are applied
- **THEN** they declare exactly one embedder, sourced as `userProvided` and not
  binary-quantized, and applying them contacts no embedding service

#### Scenario: A job with no recognised skills is still fully searchable

- **WHEN** a job whose skills are all unrecognised is indexed
- **THEN** it holds no usable vector, and the job remains findable by text and
  filterable by every facet exactly as before

### Requirement: Default ordering is newest-added first

A search request with no query text and no valid `sort` parameter SHALL return
jobs ordered by the source's posting date (`posted_at`), newest first. A request
with query text and no `sort` SHALL keep relevance order. An explicit valid
`sort` parameter SHALL always take precedence. Both `posted_at` and `created_at`
SHALL be sortable attributes of the index and accepted `sort` values. The
DB-backed jobs list keeps its own stable default (`created_at` descending) and is
no longer required to match the search default.

`sort=match` is the one accepted value that is NOT an index attribute: it orders
by the caller's own skill vector (see `skill-match-sort`). When it is served, the
request SHALL carry no attribute sort directive at all, because an explicit sort
directive takes precedence over vector ranking in the engine and would silently
discard the ordering the caller asked for.

#### Scenario: Browsing without a query shows freshest postings first

- **WHEN** the search endpoint is called with empty `q` and no `sort`
- **THEN** results are ordered `posted_at` descending

#### Scenario: A text query keeps relevance order

- **WHEN** the search endpoint is called with `q=golang` and no `sort`
- **THEN** results are in relevance order (no sort directive)

#### Scenario: Explicit sort wins

- **WHEN** the search endpoint is called with `sort=created_at&order=desc`
- **THEN** results are ordered by `created_at` descending regardless of `q`

#### Scenario: A served match sort suppresses the attribute sort

- **WHEN** the search endpoint serves `sort=match` for an eligible caller
- **THEN** the engine request carries the caller's vector and no sort directive

## ADDED Requirements

### Requirement: The jobs feed can be ordered by profile match

The system SHALL accept `sort=match` on `GET /api/v1/jobs/search`, ordering
results by the cosine between each job's skill vector and the authenticated
caller's, computed from their profile skills.

The endpoint SHALL remain public. It SHALL attach the caller's identity when a
session is present but MUST NOT reject a request that lacks one.

Ranking SHALL compose with every existing facet filter in a single engine query,
and pagination SHALL behave as it does for any other ordering — there is no
window, and no ceiling beyond the endpoint's existing pagination guard.

The caller's vector SHALL be built per request from their profile alone. It SHALL
require no database read: the vector depends on nothing but the skills the profile
names.

**`sort=match` SHALL NEVER return an error.** Every reason it cannot be served —
no session, no profile, no skills, no recognised skills —
SHALL degrade to the endpoint's default ordering. A saved search or a shared link
carrying `sort=match` must not break when opened by someone it cannot be served
for, which is the same reason the jobs list ignores unknown filters rather than
refusing them.

#### Scenario: A signed-in caller with skills is ranked by match

- **WHEN** a caller with a session and a profile naming recognised skills requests
  `sort=match`
- **THEN** the engine query carries a vector of the declared width and results are
  ordered by their cosine against it

#### Scenario: An anonymous caller gets the default feed

- **WHEN** a caller with no session requests `sort=match`
- **THEN** the response is `200` with the default freshest-first ordering, and the
  engine query carries no vector

#### Scenario: A profile with no skills gets the default feed

- **WHEN** a signed-in caller whose profile names no skills requests `sort=match`
- **THEN** the response is `200` with the default ordering and no vector

#### Scenario: The match sort composes with facet filters

- **WHEN** an eligible caller requests `sort=match` together with country and
  seniority filters
- **THEN** one engine query carries both the vector and the facet filter, and
  neither is dropped

#### Scenario: Deep pagination under the match sort behaves normally

- **WHEN** an eligible caller pages into the match-ordered feed within the
  endpoint's pagination guard
- **THEN** results continue in match order, with no separate depth limit
