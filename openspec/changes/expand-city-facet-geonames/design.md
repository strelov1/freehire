## Context

`internal/location` derives a job's geography deterministically from the free-text
`location` string. Cities live in two hand-curated maps that have drifted apart:
`nameToCountry` (~200 entries) resolves a city to a country signal, while
`nameToCity` (~100 entries) is the display facet. A city present in the first but
not the second (e.g. `florianópolis`) resolves its country yet emits no `cities`
facet value; the gap is then backfilled from the LLM's `enrichment.cities` at
serve time (`jobview.cityFacet`), which is inconsistent and absent on un-enriched
jobs.

The repo already has the generated-artifact pattern: `cmd/gen-contracts` emits a
committed file so the normal build never runs the tool. `internal/collections`
already uses `go:embed`. Deterministic facets are stored at ingest, so reaching
existing rows is the documented re-derive + reindex procedure.

## Goals / Non-Goals

**Goals:**
- Broad, deterministic city coverage (~25k GeoNames `cities15000` places) feeding
  the `cities` facet, sourced offline with no runtime dependency on the LLM.
- A single generated source of truth for city → canonical-name + country/region,
  ending the two-map divergence.
- Keep the parser's "never guess" bias: collisions and ambiguity are resolved
  conservatively at generation time.

**Non-Goals:**
- A live geocoder or fuzzy matching. Resolution stays exact alias lookup.
- Removing the LLM `enrichment.cities` serve-time fallback in the same change
  (it can stay as a last resort; the generated dictionary shrinks its role).
- Sub-city geography (districts, neighborhoods) or population thresholds below 15k.

## Decisions

**1. Dataset: GeoNames `cities15000` (pop ≥ 15k).** ~25k places — covers capitals,
IT hubs, and regional centers (Florianópolis included) with minimal noise and
collision risk. Alternatives: `cities5000`/`cities1000` give more coverage but
sharply raise common-word collisions and file size; rejected for the first cut
(the threshold is a one-line change in the generator if we later want more).

**2. Storage: `go:embed`-ed TSV, not a generated `.go` map literal.** ~25k rows
with multilingual aliases is too large for a readable Go map literal (100k+ lines,
heavy diffs/compile). A compact TSV (`canonical <TAB> country <TAB> alias|alias|…`)
is parsed once at package init into the same `map[string]…` the code already uses.
Mirrors the committed-generated-artifact convention (`cmd/gen-contracts`).

**3. Generator: `cmd/gen-cities`.** Downloads the GeoNames `cities15000.zip`,
parses the fixed-column dump, and for each place keeps name, ASCII name, native
name, alternate names, country code, and population. It (a) lowercases and dedupes
aliases, (b) for a bare name shared by multiple places keeps only the
most-populous, (c) drops aliases in the collision stoplist, and (d) writes the TSV
sorted for a stable diff. Committed output; `make gen-cities` re-runs it.

**4. City name only — never a guessed country (the key correctness decision).**
`location.Parse` resolves a city token against the merged dictionary (generated base
+ curated overrides) and emits the city's canonical display name to the `cities`
facet. It does **not** take a country/region from the dictionary: country/region
stay the curated deterministic dictionaries' job (and the LLM's, at serve time). This
honours the parser's "never guesses" contract — an ambiguous bare city (`Birmingham`,
`Valencia`, `Burlington`) can never mis-file a job under the wrong country via a
most-populous pick. When a curated token already fixed the country, the city name is
emitted only if the dictionary's country agrees (checked against that token's own
resolution, so it is order-independent), so a country/region token (`USA`) never
emits an unrelated city buried in its GeoNames alternate names (`Yokkaichi`). A
consequence: `clinch`'s slug splitter, which asks `location.Parse` whether a fragment
resolves *geography*, is unaffected — the dictionary adds no country/region, so it
needs no change. The old `nameToCity` map is replaced by the generated dictionary; a
small `cityOverrides` map pins the handful of spellings GeoNames differs on
(`Köln`→`Cologne`, `Zürich`→`Zurich`), applied over the base (override wins).

**5. Collision safety.** The stoplist excludes the parser's work-mode, open-anywhere,
and macro-region/continent markers, so a token like `Remote` or `Europe` never
becomes a city. Because the dictionary contributes no country, an ordinary word that
is also a small city can at worst add a stray *city-facet* value on a location field
that literally contains it — never a wrong country/region.

## Risks / Trade-offs

- **[A bare long-tail city yields no country/region]** → By design (never guess): a
  bare `Recife` emits the city facet but no country until an explicit country token
  or the LLM serve-time fallback fills it. Most ATS location fields carry a country/
  subdivision token, and the LLM `enrichment` geo fallback already covers the rest.
- **[Embedded dataset size]** → A ~34k-row TSV with Latin/Cyrillic aliases is ~3.3 MB,
  compiled into every binary that imports `internal/location`. Accepted: it is data
  (not code), far smaller than a Go map literal, and the idiomatic `go:embed` choice;
  workers that never call `Parse` still carry it, which is a deliberate simplicity
  trade over a lazy loader.
- **[GeoNames licensing]** → CC-BY 4.0. Attribution belongs in the generator
  header/repo, not shipped per-row.

## Migration Plan

1. Land the generator + embedded dataset + parser change; unit tests green.
2. Deploy the new binaries.
3. Run `cmd/backfill-derive` to re-derive `jobs.cities`/`countries`/`regions` over
   existing jobs, then `cmd/reindex` to refresh the Meilisearch facet — the
   standard dictionary-change procedure (no schema change, no rollback risk beyond
   reverting the binary and re-deriving).

## Open Questions

- Keep the LLM `enrichment.cities` serve-time fallback, or retire it once the
  generated dictionary lands? Default: keep it as a last resort this change.
