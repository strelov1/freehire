## Why

The `regions` reach facet mixed three taxonomic levels: continents/macro-regions
(`eu`, `apac`), single countries treated as areas (`us`, `ru`), and a vague
continent bucket (`americas`). The Americas were split across three disconnected
values — `us` (194k jobs, the country), `north_america` (Canada only — US not
included), and `americas` (172 jobs, only from literal text) — so a "North
America" filter returned no US jobs, and `us`/`ru` duplicated the separate
`countries` facet. The facet read as "не туда не сюда".

Separately, the **Skills** and **Countries** filters were free-text token inputs:
a user couldn't tell which skills or countries existed or how many jobs each had
("непонятно, есть они или нет"), and had to know exact tokens/ISO codes to type.

## What Changes

- **Collapse the region vocabulary to one consistent macro level.**
  `enrich.RegionValues` becomes `{global, north_america, latam, eu, uk, mena,
  africa, apac, cis}`. The US folds into `north_america` (with Canada); Russia,
  Belarus, Moldova, the Caucasus, and Central Asia fold into `cis`. Removed:
  `us`, `ru`, `americas`, `emea`, `eea`, `central_asia`. Country-level filtering
  lives solely in the `countries` facet. Updated `internal/location`
  (`regionCountries`, `nameToRegion`), the parser tests, the generated web
  contracts, and the curated web `REGION` filter list in lockstep.
- **Skills and Countries filters become distribution-driven selects.** They flip
  from free-text `tokens` to a searchable `select` whose options come from the
  live `GET /api/v1/jobs/facets` distribution (value → count, busiest first),
  rendered with counts so the user sees exactly which values exist. Country codes
  are labelled via `Intl.DisplayNames` (no hand-maintained table). The job-search
  view fetches the distribution (debounced, with a stale-response guard) and
  threads it through the filter panel.
- **Pill facets surface selected-but-unknown values.** A selected value with no
  matching option (e.g. a removed region code in an old bookmark/saved search)
  renders as a removable pill instead of becoming an invisible, stuck filter.

A deploy of this change requires `cmd/backfill-derive` (re-derive every job's
facets under the new vocabulary) + `make reindex` to reach the live data and the
search index; until then existing jobs keep their old region codes.

## Capabilities

### Modified Capabilities
- `job-geography`: the controlled region vocabulary is now a single macro level;
  country codes are no longer emitted as regions.
- `web-frontend`: open/high-cardinality facet filters (skills, countries) are
  driven by the live facet distribution with counts, not free-text entry; pill
  facets keep removed-vocabulary selections removable.
