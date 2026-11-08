## Context

Job geography drives the most-used filter, but it is nearly empty. Of ~72k jobs
only 4% are enriched, and `enrichment.regions` is filled for just 16% of remote
jobs. A prod investigation showed the geographic signal is already present in the
structured `location` field for ~90% of remote jobs lacking regions
(`Remote - USA`, `Remote - UK`, `United States`, …); the LLM drops it because the
enrichment prompt forbids inference ("never guess"). Today `regions` lives only
in the `jobs.enrichment` JSONB (an AI-derived blob) and is faceted by Meilisearch
via the `enrichment.regions` dot path.

Constraints: sqlc is the only DB layer (edit `queries/*.sql`, run `make sqlc`);
migrations apply only on first volume init, so prod gets them by manual `psql`;
search-field changes require a reindex; the SPA has no test runner (verify via
svelte-check + lint); enrichment (`SetJobEnrichment`) and ingest (`UpsertJob`)
are deliberately decoupled write paths.

## Goals / Non-Goals

**Goals:**

- Populate `countries` + `regions` for every job deterministically from its
  `location` string, independent of enrichment.
- Redefine `regions` as the geographic area of a job (any work mode), so the
  facet is useful for onsite/hybrid roles too.
- Present geography as a single top-level facet that unions the ingest-derived
  values with whatever the LLM additionally found in the description body.
- Backfill existing rows so the facet is immediately populated, including closed
  jobs that never re-crawl.

**Non-Goals:**

- A full geocoder. The parser is a curated dictionary seeded from real prod
  locations; unresolvable single cities are omitted, not guessed.
- Changing `work_mode`/`remote` detection — that stays the LLM's job.
- Postgres-side region filtering / GIN indexes — faceting stays in Meilisearch.
- Materializing the union into the DB.

## Decisions

**1. Parse at ingest, not (only) at enrichment.** The location field is short and
structured-ish, so a deterministic parser hits ~100% where the LLM hit 16%, runs
on all jobs (not just the enriched 4%), and costs no LLM latency. The LLM is kept
as an additive source for reach stated only in the description body.
*Alternative — fix the prompt:* rejected; still gated on the enrichment backlog
and the model's conservatism, and non-deterministic.

**2. Union at READ time in `jobview.FromRow`, not materialized.** Ingest
(`UpsertJob`) and enrichment (`SetJobEnrichment`) are separate writers; a
materialized union would need both to keep it in sync — exactly the coupling the
codebase avoids. `jobview` already decodes enrichment on read, so the union is a
natural extension there. `FromRow` computes `jobs.regions ∪ enrichment.regions`
(and countries) into new top-level `Regions`/`Countries`, then blanks
`enrichment.regions`/`enrichment.countries` in the *served* copy so geography is
reported once (the stored JSONB is untouched — the LLM data stays intact).
*Alternative — pre-fill `enrichment.regions` at ingest:* rejected; pollutes the
"AI-derived" blob and creates write-path precedence conflicts.

**3. New `internal/location` package owning a hand-curated dictionary.**
`Parse(location string) (countries, regions []string)` over three maps:
`nameToCountry` (lowercase name / ATS shorthand / beacon city → ISO alpha-2),
`nameToRegion` (macro names → region, e.g. `europe→eu`, `worldwide→global`), and
`countryToRegion` (ISO → region from the existing `enrich.RegionValues` vocab).
Tokenize on `, ; / |`, ` - `, ` or `. Emit only values in `RegionValues` / valid
ISO, deduped. Bare `Remote` with no country → empty (`global` only from explicit
anywhere/worldwide). Output values are constrained to the same vocabulary the
search facet uses, so they need no further validation.
*Alternative — a countries library:* rejected; libs match formal names, not ATS
shorthand ("USA", "Remote - UK"), and still need our `ISO → region` map.

**4. Geography columns are source facts on `jobs`, written by `UpsertJob`.** They
sit beside `location`/`remote` and follow its INSERT + `ON CONFLICT DO UPDATE`
pattern, so a re-crawl refreshes them. This is distinct from the enrichment
columns `UpsertJob` deliberately never writes.

**5. Backfill via a run-once `cmd/backfill-geo`.** Keyset scan
(`ListJobsByIDAfter`), `location.Parse`, write through a new `SetJobLocation`
query. Deterministic and idempotent — re-running converges. Follows the existing
run-once worker/command pattern (enrich, reindex, reslug).

## Risks / Trade-offs

- **Facet semantics change is breaking** → A `regions` filter now also matches
  onsite offices, not just remote reach. This is the intended new meaning;
  documented in the spec and the proposal. The SPA filter label/behavior follows.
- **Dictionary coverage is partial at launch** → Seed from the real prod location
  distribution (high-frequency strings first); unresolved locations simply yield
  no geography (no wrong data). Dictionary grows by observation — note the seam.
- **Search facet move requires reindex** → Without it, `/jobs/search` filters on a
  stale attribute. Mitigation: reindex is a known post-deploy step; call it out in
  the migration plan.
- **Prod migration is manual** → Adding two columns must be applied by hand on the
  existing volume before the new code filters on them. Apply migration first, then
  deploy, then backfill, then reindex.

## Migration Plan

1. Add migration (`jobs.countries`, `jobs.regions`), `make sqlc`, ship code.
2. On prod: apply the migration manually via `psql` against the existing volume.
3. Deploy the new image (ingest starts populating geography on each crawl).
4. Run `cmd/backfill-geo` once to populate existing rows (incl. closed).
5. Run `reindex` to update Meilisearch filterable attributes and re-push docs.
Rollback: the columns are additive and default `'{}'`; reverting the code leaves
them unused and harmless. The facet falls back to `enrichment.regions` only if
the search settings are also reverted + reindexed.

## Open Questions

None — design approved in brainstorming.
