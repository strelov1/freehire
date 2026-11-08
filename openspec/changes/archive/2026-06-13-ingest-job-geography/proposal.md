## Why

Geography is the facet users filter on most, yet it is almost empty: only 4% of
jobs are enriched, and among those only 16% of remote jobs carry a `regions`
value. Prod evidence shows the signal is already present in the structured
`location` field — 90% of remote jobs missing regions have geo text like
`Remote - USA`, `Remote - UK`, `United States` — but the LLM under-extracts it
because its prompt is deliberately conservative ("never guess"). Parsing the
location string deterministically is more reliable (100% vs 16%), covers all
~72k jobs immediately instead of waiting on a 26‑day enrichment backlog, and
adds zero LLM latency.

## What Changes

- Derive job geography (`countries` + `regions`) deterministically from the ATS
  `location` string during ingest, and persist it on `jobs`.
- Redefine `regions` from "remote reach" to **the geographic area of the job**,
  meaningful for every work mode (so a `regions = eu` filter matches both
  remote‑EU roles and offices in Berlin). **BREAKING** for the existing
  `enrichment.regions` facet semantics.
- Union the ingest‑derived geography with the LLM‑derived
  `enrichment.regions`/`countries` at read time and expose it as a **top‑level**
  job facet (`regions`, `countries`), reported once.
- Move the search region/country facet from `enrichment.regions` to the
  top‑level merged fields; the SPA filters on the new fields.
- One‑off backfill of the new columns over existing rows from their stored
  `location` text.

## Capabilities

### New Capabilities

- `job-geography`: the deterministic `location` → (`countries`, `regions`)
  parser, its controlled output vocabularies (reusing the enrichment region
  vocabulary + ISO‑3166 country codes), the read‑time union with LLM‑derived
  geography, and the public top‑level geography contract.

### Modified Capabilities

- `source-ingest`: the ingest write path now parses the location string and
  persists `jobs.countries`/`jobs.regions` alongside the raw posting.
- `job-enrichment`: `regions` is redefined as geographic area (any work mode),
  and `enrichment.regions`/`enrichment.countries` are no longer served as
  independent fields — they fold into the top‑level union; the enrichment prompt
  drops the "only when remote" restriction.
- `job-search`: the region/country facet filters on the top‑level merged
  geography instead of `enrichment.regions`.

## Impact

- **Schema**: new migration adds `jobs.countries text[]` and `jobs.regions
  text[]` (manual apply on prod — migrations run only on first volume init).
- **Code**: new `internal/location` package; `internal/pipeline` (normalize),
  `internal/db/queries/jobs.sql` (`UpsertJob`, new `SetJobLocation`),
  `internal/jobview`, `internal/search` (document + filterable attributes),
  `internal/enrich/langchain.go` (prompt), new `cmd/backfill-geo`.
- **Search**: Meilisearch filterable‑attributes change → reindex required after
  deploy.
- **Web**: SPA region filter maps to the top‑level `regions` field
  (verified via svelte-check + lint; no SPA test runner).
