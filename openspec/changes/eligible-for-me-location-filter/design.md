## Context

See `proposal.md` - Why/What Changes for motivation. Relevant existing pieces this
design builds on, all already in the repo:

- `internal/search/search/query_filter.go`: `regions`/`countries` are already ORed
  as one location facet group, and `regions=none` (`RegionUnspecified`) already
  means "empty regions array" via `IS EMPTY`. Nothing here changes.
- `web/src/lib/facets.ts` `FacetStore`: `add`/`remove`/`facet` already give pure,
  URL-synced staging of include values — the same primitives `filtersFromProfile`
  (`web/src/lib/facetModel.ts`) uses for "Apply my profile."
- `web/src/lib/geoScope.ts` + `web/src/routes/geo/region/+server.ts`
  (`geo-default-scope`): already resolves a visitor's macro-region from the
  `CF-IPCountry` edge header, already excludes crawlers, already answers
  `private, no-store`. Built for the jobs feed's one-time opening-scope offer
  (`JobsView.svelte`'s `offerGeoScope`), which is a *separate* mechanism (a
  dismissible one-time URL rewrite, gated by `shouldOfferGeoScope`'s
  search/stored-filters/offered precedence) that this change does not touch.
- `internal/identity/userprofile.LocationPreferences.Base.Country`: already
  validated ISO alpha-2, already served on `/me/profile`, already read by
  `filtersFromProfile`.

## Goals / Non-Goals

**Goals:**
- Ship the "Eligible for me" pill exactly as specified in
  `specs/eligible-for-me-filter` and `specs/filter-modal`.
- Reuse `geoScope.ts`/`/geo/region` for the region-level fallback instead of
  building a second geography resolver.

**Non-Goals:**
- No manual country picker or client-side geography cache — superseded by reusing
  `geo-default-scope`, per the discussion in this change's history.
- No write-back to the candidate's profile from this filter action.
- No change to `JobsView.svelte`'s existing one-time opening-scope offer — it keeps
  its own guard logic (`shouldOfferGeoScope`) and stays independent of this
  persistent, explicit pill.
- No change to the candidate profile's `location_preferences` edit form (deferred
  follow-up, out of scope here).

## Decisions

**A small pure module resolves the geography source and the value set to stage.**
Colocated near `filtersFromProfile` in behavior (pure, testable without mounting a
component), it exposes:
- A precedence function `(profileCountry: string | null, edgeRegion: string | null) => GeoSource`
  where `GeoSource` is `{ kind: 'country'; code: string } | { kind: 'region'; code: string } | null`,
  implementing the two-step precedence from the spec.
- A function from `GeoSource` to the facet values to stage: `{ countries: [code] }`
  for `kind: 'country'`, `{ regions: [code, 'global', 'none'] }` for `kind: 'region'`,
  always folding in `global` and `none`.
- An "is currently on" predicate reading the live `FacetStore` state against the
  resolved `GeoSource`, so the pill's on/off rendering is derived from the URL
  (survives reload/back-nav) rather than tracked as separate component state.

Alternative considered: track on/off as local component `$state` in `LocationPane`.
Rejected — the existing region/country/city chips are already URL-derived, and a
component-local flag would drift from the URL on reload or when a chip is removed
by hand (the spec requires the pill to reflect that).

**`LocationPane` fetches `/geo/region` itself, lazily, only when needed.** The pane
already only mounts when the filter modal is open (`FilterModal.svelte`'s `$effect`
pattern for warming other data), so this is not an initial-page-load cost. It fetches
only when the signed-in profile (once loaded) has no `base.country` — if a profile
country is already known, the edge call is skipped entirely.

Alternative considered: reuse `JobsView.svelte`'s already-fetched `guessedRegion`.
Rejected — that value is scoped to the one-time-offer flow (cleared once the offer
resolves, not exposed as shared state) and coupling the pill to it would tie two
independently-speced behaviors together for a saving of one small, `no-store`
request that only happens when a visitor actually opens the filter modal.

**The pill is a single on/off toggle, not a cycling chip.** The existing
region/country/city chips cycle include → exclude → off, which does not fit
"eligible for me" (there is no meaningful "exclude what's eligible for me" state).

## Risks / Trade-offs

- **Two independent `/geo/region` calls can happen in one session** (the opening-scope
  offer in `JobsView.svelte`, and this pill if the modal is opened) → both are cheap,
  `private, no-store` by design, and only fire under their own distinct guards; not
  worth sharing state for the volume involved.
- **A profile that loads after the pane has already rendered a region-level pill**
  could flicker to country-level once it arrives → acceptable given `FilterModal`
  already gates profile warm-up on `open`, so the common case (profile loaded before
  the visitor opens the modal) sees no flicker; not fixed further here.
- **A visitor who removes only one of the three staged values by hand** (e.g. the
  `Not specified` chip) leaves the pill showing "off" even though most of its effect
  remains → intended per the spec's "reflects exact staged state" requirement, and
  consistent with how every other chip in the pane already behaves.
