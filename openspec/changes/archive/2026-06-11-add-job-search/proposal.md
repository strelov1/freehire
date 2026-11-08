## Why

The only way to find jobs today is `GET /api/v1/jobs` — offset pagination with no
keyword query and no faceted filtering. A job aggregator's core value is *finding*
the right role fast: full-text matching on title/company/description, typo
tolerance, and filtering on the structured facets the AI enrichment already
produces (seniority, category, work mode, salary, countries, skills…). Postgres
`ILIKE`/`tsvector` would get us keyword matching but not ergonomic faceting,
typo tolerance, or semantic ranking. A purpose-built search engine does.

## What Changes

- Add **Meilisearch** as a search backend (new `meilisearch` service in Docker
  Compose; new `internal/search` package built on the official `meilisearch-go`
  SDK). Configured by `MEILI_URL` + `MEILI_MASTER_KEY`.
- Define a `jobs` index: one document per job (primary key = job `id`), with
  searchable attributes (title, company, description, location), filterable
  facets drawn from the raw row and the enrichment JSONB (source, remote,
  company_slug, work_mode, employment_type, seniority, category, domains,
  countries, company_type, company_size, visa_sponsorship, salary_currency,
  salary_period, skills, salary_min, salary_max, experience_years_min), and
  sortable attributes (posted_at, salary_min, salary_max).
- **Hybrid search from day one**: configure a Meilisearch embedder with source
  `huggingFace` (a multilingual MiniLM model that runs *inside* Meilisearch — no
  external API key). Queries blend keyword and semantic ranking via a
  `semanticRatio`; `semanticRatio=0` is pure keyword and still works.
- New public endpoint **`GET /api/v1/jobs/search`** (`q`, facet filters, sort,
  pagination, optional `semantic_ratio`) returning the standard
  `{"data": [...], "meta": {...}}` envelope. The existing `GET /api/v1/jobs`
  list is untouched.
- New batch command **`cmd/reindex`** (mirrors `cmd/enrich`: `config.Load` +
  `database.Connect` + a `Runner`) that scans `jobs` from Postgres and pushes
  documents to Meilisearch in batches; run on a schedule. Documents store enough
  fields to render results directly (no DB rehydrate).
- Docker Compose gains the `meilisearch` service + volume; the Makefile gains a
  `reindex` target.

## Capabilities

### New Capabilities

- `job-search`: how jobs are searched — the Meilisearch-backed `jobs` index
  (document shape, searchable/filterable/sortable attributes, the hybrid
  embedder), the public `GET /api/v1/jobs/search` contract (query, facet
  filters, sort, pagination, semantic blend), and the batch reindex that keeps
  the index in sync with Postgres.

### Modified Capabilities
<!-- No existing capability owns the public job read/list endpoints (no `jobs`
     spec under openspec/specs/), and `GET /api/v1/jobs` is unchanged, so this
     is purely additive. -->

## Impact

- **New dependency**: `github.com/meilisearch/meilisearch-go` (+ a `meilisearch`
  container in Docker Compose; `MEILI_URL`/`MEILI_MASTER_KEY` config).
- **New code**: `internal/search` (client, index settings/ensure, document
  mapping, index/search/delete helpers); `cmd/reindex` (batch sync `Runner`);
  a search handler in `internal/handler`.
- **DB access**: a streaming/batched read of `jobs` for reindex (a
  `ListJobsForIndex` query or reuse of paginated `ListJobs`); regenerate
  `internal/db` via `make sqlc` if a new query is added.
- **Config / infra**: `internal/config` Meilisearch settings; `docker-compose.yml`
  service + volume; `Makefile` `reindex` target.
- **SPA**: a thin search box on the web frontend wired to the new endpoint
  (small follow-up task within this change).
- **Out of scope**: real-time/outbox indexing (a `search_outbox` table is noted
  as a seam, analogous to `enrichment_outbox`, but NOT built); ranking-rule
  tuning beyond Meilisearch defaults; search analytics.
