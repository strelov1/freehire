## Why

The catalogue is browsable only by deterministic facets (skills, geography, seniority, source). There's no way to surface jobs by an editorial *theme about the company* — "YC-backed companies", "Big Tech". These themes are facts about the company (who funded it, how prominent it is), not properties derivable from a job's text or its ATS source (e.g. Airbnb/Dropbox are YC but hire through their own Greenhouse, not Work at a Startup). Curated collection pages turn these into shareable, SEO-friendly landing pages over the existing job feed.

## What Changes

- Introduce **collection membership** as a company-level fact: a company can belong to one or more curated collections (e.g. `yc`, `techstars`, `european`, `ai`, `mag7`, `bigtech`, `unicorn`, `fortune500`).
- Add `companies.collections` (membership) and `jobs.collections` (denormalized copy for the search facet), mirroring how `company_slug` is denormalized onto jobs.
- Index `collections` as a filterable Meilisearch facet so a collection reuses the existing faceted job search unchanged.
- Add a static collection registry (slug, title, description, membership source); each entry's source is either a hand-coded slug list or a remote dataset (URL + parser). Adding a collection is one registry entry.
- Add `cmd/import-collections`: an idempotent run-once worker that resolves each collection's members (a fetched dataset matched by normalized name, or a hand-coded slug list), writes `companies.collections`, propagates to `jobs.collections`, and prompts a reindex; it aborts before writing if any dataset fails to resolve.
- Expose `collections` as a selectable facet in the `/jobs` filter sidebar (composable with every other facet) and add a `/collections` discovery hub (the fixed set with open-job counts) that links into `/jobs?collections=<slug>`, plus a nav link. There is no separate per-collection page — the facet is the single rendering of a collection's jobs.

## Capabilities

### New Capabilities
- `job-collections`: curated, company-themed collections — membership model, the deterministic propagation onto jobs, the search facet, the import worker, and the `/collections` web pages.

### Modified Capabilities
- `job-search`: the served job wire shape and the search document gain a `collections` field, and `collections` becomes a filterable facet/query parameter.

## Impact

- **Schema**: new migration adding `companies.collections` and `jobs.collections` (both `TEXT[]`).
- **Code**: new `internal/collections` (registry + matching), new `cmd/import-collections`; touches `internal/jobview`, `internal/search` (document + filterable attributes + query-param map), and the Dockerfile (new binary).
- **Search**: requires a reindex after each import to pick up `jobs.collections` (same operational caveat as every other dictionary facet).
- **Web**: new `web/src/routes/collections` pages + a contracts mirror of the collection registry.
- **Data quality**: YC membership is matched by normalized name — coverage is partial (unmatched companies are simply omitted; rare same-name false positives possible). Accepted for a curated showcase; domain-based matching is a later refinement.
