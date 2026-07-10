## Why

Loxo hosts the public careers boards of many recruiting agencies (e.g. FitNext
Consulting, Agile Recruiter, LA-Tech), each carrying tech vacancies that
freehire does not yet aggregate. A spike VALIDATED that these boards are cleanly
scrapeable and that the pattern generalizes across agencies and host variants, so
a single Loxo adapter unlocks a whole class of sources at once.

## What Changes

- Add a `loxo` ATS adapter (`internal/sources/loxo.go`) in the careerspage style:
  crawl a board's SSR careers page, extract `/job/<base64>` links, fan out to each
  detail page, and map it to a `Job`. One line in `sources.All`.
- Honor the existing `CompanyEntry.Hub` flag for employer attribution (Loxo boards
  are agencies whose vacancies belong to different clients): resolve the client from
  the posting when explicitly present, else fall back to the agency name — never guess.
- Add `cmd/harvest-loxo`, a discovery prober that enumerates Loxo agency boards from
  the search-engine footprint, live-validates each, and emits draft
  `sources/loxo.yml` entries for human curation (mirrors `cmd/harvest-boards`).
- Add the `sources/loxo.yml` board file (all entries `hub: true`), seeded with
  the validated agencies from the spike.
- No new tech-gating or lifecycle mechanism: Loxo is a registered board source, so
  it reuses the per-provider unseen sweep and the existing per-job non-tech gate.

## Capabilities

### New Capabilities
- `loxo-source`: the Loxo careers-board adapter — listing crawl, detail parse
  (title, embedded-JSON description, stable `<agency_id>-<slug>` external id,
  best-effort location/remote), and Hub-based employer attribution.
- `loxo-harvest`: the `cmd/harvest-loxo` discovery prober — footprint enumeration,
  live validation, tech-relevance counting, and draft board-file emission.

### Modified Capabilities
<!-- None: Hub is an existing field already honored by huntflow; the unseen-sweep
     and non-tech gate are reused unchanged, so no spec-level behavior changes there. -->

## Impact

- New code: `internal/sources/loxo.go` (+ test + fixtures), `cmd/harvest-loxo/main.go`,
  `sources/loxo.yml`; one registration line in `internal/sources/source.go` (`All`).
- Reuses: careerspage-style `HTMLGetter`, `CompanyEntry.Hub` (huntflow precedent),
  `internal/classify` (harvester tech count), the ingest unseen sweep.
- Accepted trade-off (seam, no mitigation now): cross-source duplicates — an agency
  repost of a job the client also posts directly to its own ATS yields a second row,
  since freehire dedups only within a `source`. Treated as a legitimately distinct
  listing.
