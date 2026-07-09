## Why

The `cities` search facet is sparse and inconsistent. Two hand-curated maps in
`internal/location/dictionaries.go` diverge: `nameToCountry` (~200 entries) lists
cities as country signals, but `nameToCity` (~100 entries) is the display facet —
so a city like `Florianópolis` resolves its country (`br`) yet contributes nothing
to the city facet. Everything outside the small `nameToCity` map depends on the
LLM's `enrichment.cities` at serve time, which is inconsistent in spelling and
coverage and absent on un-enriched jobs. We want broad, deterministic city
coverage sourced from an authoritative dataset (GeoNames), so the facet is
consistent without leaning on the LLM.

## What Changes

- Add a build-time generator (`cmd/gen-cities`) that downloads the GeoNames
  `cities15000` dump (population ≥ 15k, ~25k cities; CC-BY licensed), filters and
  normalizes it, and writes a compact embedded dataset (a `go:embed`-ed TSV of
  `canonical-name <TAB> country-code <TAB> alias|alias|…`).
- `internal/location` loads the embedded dataset once at init and resolves a city
  token against it, emitting the city's canonical display name to the `cities`
  facet. The generated dictionary supplies the **city name only** — country and
  region still come from the curated deterministic dictionaries (and the LLM at
  serve time), so an ambiguous city name never *guesses* a geography (honouring the
  parser's "never guesses" contract). The old `nameToCity`↔`nameToCountry`
  divergence is closed by making the generated dictionary the single, broad source
  of city facet values.
- Disambiguation and collision safety are part of the generated data: a shared
  alias resolves to its most-populous GeoNames match (a deterministic display
  name); names that collide with work-mode / open-anywhere / macro-region markers
  are excluded (never guess).
- The hand-curated city entries that GeoNames does not cover (ATS shorthands,
  campus names like `Cupertino`) are retained as explicit overrides layered on the
  generated base.

## Capabilities

### New Capabilities
- `city-dictionary`: A GeoNames-derived, build-time-generated, embedded city
  dictionary and its generator (`cmd/gen-cities`) — coverage threshold,
  multilingual aliases, canonical display name, most-populous disambiguation, and
  the common-word collision stoplist.

### Modified Capabilities
- `job-geography`: The deterministic location parser resolves cities from the
  generated dictionary and emits the `cities` facet value; country/region stay the
  curated dictionaries' job (never guessed from an ambiguous city), replacing the
  divergent hand-curated `nameToCity` map with a broad generated source.

## Impact

- New: `cmd/gen-cities/main.go`, an embedded dataset file under
  `internal/location/`.
- Modified: `internal/location/location.go`, `internal/location/dictionaries.go`
  (city maps become generated + curated overrides).
- Deterministic facets are stored on ingest, so reaching existing jobs needs a
  re-derive (`cmd/backfill-derive`) + `cmd/reindex` — the standard dictionary-change
  procedure; no schema change.
- No API shape change: `cities` stays a top-level facet on the job object and a
  Meilisearch filterable attribute.
