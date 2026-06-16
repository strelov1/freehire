## Why

The six dictionary-derived facets (countries, regions, work_mode, skills, seniority, category) are produced both by the deterministic curated dictionaries (`jobderive`) and by the LLM enrichment, and `jobview.FromRow` currently lets the LLM contribute to them — unioning geography/skills and letting the LLM override work_mode/seniority/category. This couples production facets to the LLM, so the LLM cannot be loosened into a free-form discovery signal without corrupting served data. Making the deterministic dictionaries the single source of these six facets is the prerequisite that unblocks a later "relax the LLM for discovery" change.

## What Changes

- **BREAKING (served data):** `jobview.FromRow` sources the six facets from the deterministic `jobs` columns only:
  - `countries` / `regions` / `skills` — served from `j.*` only (the union with the LLM enrichment values is removed).
  - `work_mode` — served from `j.WorkMode` only (the LLM no longer overrides).
  - `seniority` / `category` — overwritten by `j.Seniority` / `j.Category` (the dictionary always wins; the LLM is no longer a fallback).
- The raw LLM values for these six fields remain stored in the `jobs.enrichment` JSONB, untouched — they are simply no longer served (future discovery material).
- Because `FromRow` is the single merge chokepoint feeding both the API and the search index, the Meilisearch facets become dict-only automatically (a reindex is required after deploy).
- The three backfill workers (`cmd/backfill-geo`, `cmd/backfill-skills`, `cmd/backfill-class`) and their per-column sqlc queries are **removed** and replaced by one `cmd/backfill-derive` that calls `jobderive.Derive` and rewrites all six facet columns in a single pass (slugs untouched), via one new combined sqlc `UPDATE`. The Dockerfile build/COPY list and the `freehire-ops` cron are updated accordingly.

## Capabilities

### New Capabilities
- `deterministic-facets`: The doctrine that the six dictionary-derived facets are sourced solely from the deterministic dictionaries (`jobderive`) at read time, the LLM's values for those six are excluded from the served wire shape (kept raw in the enrichment JSONB), and existing jobs are re-derived for all six facets by a single unified backfill pass.

### Modified Capabilities
- `job-geography`: The read-time merge requirement changes from "ingest-derived and enrichment-derived geography are **unioned**" to dict-only; the work-mode read-time precedence changes from "the LLM work mode beats the ingest value" to dict-only (ingest/parser value only); the "existing jobs are backfilled with parsed geography" requirement generalizes to the single unified `backfill-derive` pass.

## Impact

- Code: `internal/jobview/jobview.go` (merge logic), `internal/jobview/jobview_test.go`; new `cmd/backfill-derive/main.go`; removed `cmd/backfill-geo`, `cmd/backfill-skills`, `cmd/backfill-class`; `internal/db/queries/*.sql` (one new combined update, three removed); regenerated `internal/db`.
- API/search: the served `countries`/`regions`/`work_mode`/`skills`/`seniority`/`category` facets become dict-only; a `reindex` is required post-deploy so the Meilisearch index reflects the new sourcing.
- Ops: `Dockerfile` (−3 binaries, +1); `freehire-ops` cron (collapse three backfill jobs into one); deploy tail = run `backfill-derive` then one `reindex`.
- Out of scope: relaxing the LLM, discovery capture, dictionary enrichment, normalization tooling, and all LLM-only enrichment fields (salary, employment_type, english_level, education_level, domains, company_type/size, relocation, visa).
