# Location conventions

## Scope
Curated dictionary deriving ISO 3166-1 alpha-2 country codes, region codes, and a work-mode hint from a free-text ATS location string.

## Always true
- It is a curated dictionary, not a geocoder — it resolves high-frequency names/shorthands and emits nothing for what it can't resolve (never guesses).
- Region and work-mode values are drawn from the same controlled vocabulary the enrichment contract defines (`enrich.RegionValues`/`enrich.WorkModeValues`), so the parser, the enrichment payload, and the search facet all speak one set of values.
- Geography is exposed as a Meilisearch facet (`regions`/`countries`/`work_mode` are filterable attributes), not a Postgres column filter.
- A dictionary change needs a re-derive (`cmd/backfill-derive`) and a `cmd/reindex` to reach existing jobs.
- `work_mode` is dict-only — `jobview` serves the `jobs` column alone, the LLM's `enrichment.work_mode` is never merged in (it stays raw in the JSONB).
- `countries`/`regions` are a dict-then-LLM hybrid (`jobview.geoFacet`): the dictionary wins when it pins a place, and the LLM's `enrichment.countries`/`regions` fill only the unpinned (global/unspecified) bucket, so a dictionary-silent remote role still gets a geographic reach rather than none. This is the one deliberate exception to dict-only among the facets.

## How it works
`internal/location` parses the free-text location string from an ATS posting (e.g. "Berlin, Germany" or "Remote - US") and derives structured geography from it. The dictionary is curated for high-frequency names and shorthands; anything it cannot resolve is left empty rather than guessed. Because geography lives as Meilisearch facets (not Postgres column filters), the dictionary output flows through the search index, not through SQL WHERE clauses. This means a dictionary update is a two-step propagation: re-derive the facet columns on existing jobs (`cmd/backfill-derive`), then rebuild the search index (`cmd/reindex`). The production facets follow a deliberate split: `work_mode` is purely dict-driven (the LLM's work-mode guess is never served), while `countries`/`regions` are a hybrid where the dictionary pins what it can and the LLM fills the gaps for remote/unspecified roles.

## Limitations
None currently listed.
