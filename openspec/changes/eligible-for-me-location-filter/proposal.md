## Why

Job search issue #2709: a candidate in Indonesia found a job listed as remote but
actually restricted to Canada, and had to read the full posting to discover it wasn't
open to them. The search API already carries everything needed to tell them apart
(`jobs.countries`, `jobs.regions`), but nothing in the UI lets a visitor say "show me
only what I'm plausibly eligible for" in one action — they would have to manually
combine a country pill and a region pill, a combination nothing hints at.

Production data measured this session (open remote jobs, n=348,805) shows why this
needs care rather than a blanket default: 77.7% are pinned to a single country, only
9.9% are truly worldwide, and 11.2% carry no resolved geography at all (a location
string the dictionary couldn't parse). A naive "only show my country" filter would
either make the catalogue look broken for most visitors (if auto-applied) or silently
hide the 11.2% unresolved-geography bucket as if it were "not for me" (if it ignores
that bucket).

## What Changes

- Add an opt-in **"Eligible for me"** pill to the job search Location pane
  (`web/src/lib/components/filters/LocationPane.svelte`), next to the existing
  `Global` / `Not specified` flat pills.
- Toggling it on stages three *already-existing, already-supported* facet include
  values in the location filter group (no backend change — `internal/search/search/query_filter.go`
  already ORs `regions`/`countries` together):
  - `countries=<code>` when a specific country is known (signed-in profile's
    `location_preferences.base.country`);
  - `regions=<macro-region>` when only a macro-region is known (reusing the existing
    `geo-default-scope` CF-IPCountry → region resolution, `web/src/lib/geoScope.ts` /
    `/geo/region`, for every visitor who has no profile country — anonymous included,
    no manual picker);
  - `regions=global` and `regions=none` (the existing `RegionUnspecified` sentinel)
    always, so worldwide-remote jobs and jobs whose geography never resolved stay
    visible rather than being swept into "not eligible."
- Toggling it off removes exactly those staged values.
- The control is strictly opt-in — never applied by default or automatically — given
  how large the country-pinned share of the catalogue is.
- Not restricted to `work_mode=remote`: since `countries`/`regions` already filter
  independently of work mode, the toggle narrows onsite and remote postings alike to
  "geographically plausible for me."

## Capabilities

### New Capabilities
- `eligible-for-me-filter`: the toggle's behavior — which facet values it stages, the
  country/region source precedence (profile country → CF-IPCountry-derived region →
  unavailable), the always-included `global`/`none` safety values, and the strictly
  opt-in default.

### Modified Capabilities
- `filter-modal`: the Location pane gains the "Eligible for me" pill alongside the
  existing `Global` / `Not specified` flat pills.

## Impact

- **Frontend only.** `web/src/lib/components/filters/LocationPane.svelte` (new pill +
  interaction), a small pure function for the on/off state and the value set to
  stage/unstage (colocated near `filtersFromProfile` in `web/src/lib/facetModel.ts`,
  or as its own module), and reuse of `web/src/lib/geoScope.ts` /
  `web/src/routes/geo/region/+server.ts` (already built for `geo-default-scope`, no
  changes needed there).
- No backend, database, or search-index changes — `countries`, `regions`, and the
  `regions=none` sentinel already exist and are already ORed as one location group.
- Out of scope: the candidate profile's `location_preferences` edit form (a separately
  noted follow-up the user deferred) and any change to `internal/dict/location`.
