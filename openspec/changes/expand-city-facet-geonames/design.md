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

**4. One resolution path.** `location.Parse` resolves a city token against the
merged dictionary (generated base + curated overrides). A hit writes the canonical
name into the city set AND the country code into the country/region sets — the two
former maps collapse into one lookup. The curated overrides (`nameToCity` entries
GeoNames lacks: `Cupertino`, ATS shorthands) are a small literal map applied over
the generated base at init; an override wins on key collision.

**5. Collision safety.** The stoplist = a small curated list of common
English/other words that are also GeoNames place names (`Of`, `As`, `Mobile`,
`Reading`, `Remote`, …) plus the parser's existing work-mode and open-anywhere
markers, so a city never misfires from an ordinary token or a work-mode word.

## Risks / Trade-offs

- **[Ambiguous bare names still pick one country]** → Most-populous wins; this is
  the same bias the current `nameToCountry` uses (it lists the well-known city).
  A location that means a smaller same-named town is mis-countried — acceptable,
  and no worse than today.
- **[Common-word collisions inflate false cities]** → The stoplist + the ≥15k
  threshold + exact (not fuzzy) matching keep this small; the stoplist is curated
  from an actual scan of `cities15000` names against a common-word list.
- **[Embedded dataset size]** → A ~25k-row TSV with aliases is on the order of a
  few hundred KB, compiled into the binary. Acceptable; far smaller than a Go map
  literal of the same data.
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
