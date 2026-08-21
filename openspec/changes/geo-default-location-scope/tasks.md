## 1. Country at the edge

- [x] 1.1 Write the country→region normalizer as a pure function: **lower-case** the code (the edge sends `BR`, the generated grouping is keyed `br`), drop `XX` and `T1`, drop anything `COUNTRY_REGION_MAP` does not carry, return a region value or nothing
- [x] 1.2 Unit-test it: a placeable country, `XX`, `T1`, a lower-case code, an unknown code, and a missing header
- [x] 1.3 Add the `+server.ts` endpoint that reads `CF-IPCountry`, runs the normalizer, and answers `{ region }`
- [x] 1.4 Set `Cache-Control: private, no-store` on that response
- [x] 1.5 Answer a recognized crawler user agent with no region
- [x] 1.6 Confirm no route's server `load` reads the header — the region must reach the client only through this endpoint. No nginx entry needed: host2 sends `location /` to SvelteKit, and only Go routes outside `/api/` need an ops snippet

## 2. The once-per-browser marker

- [x] 2.1 Add the marker key and its load/save helpers to `web/src/lib/filterStorage.ts`, wrapped exactly like `JOB_FILTERS_KEY` (feature-detect `localStorage`, swallow failures)
- [x] 2.2 Make an unreadable or unwritable store report "already offered", so the guess never runs when it cannot be recorded
- [x] 2.3 Unit-test that `saveJobFilters('')` — the clear path — leaves the marker in place
- [x] 2.4 Unit-test the storage-throws path for both helpers

## 3. Applying the derived scope

- [x] 3.1 Extend the `afterNavigate` block in `web/src/lib/components/JobsView.svelte` (around the existing `loadJobFilters()` restore) with the derived scope as the last branch, keeping the precedence order from design.md
- [x] 3.2 Call the region endpoint only from that last branch, so a visitor with URL geography, a stored set, or the marker already set never makes the request
- [x] 3.3 Apply it as the `regions` facet holding the derived region **and** the worldwide region
- [x] 3.4 Write the URL and re-seed from it, then set the marker. **Changed from the plan:** shallow `replaceState` + `filters.syncFromUrl()`, not `goto` — `filters.apply()` would have persisted the guess, and the `enter` objection to `replaceState` expires once a network round trip has passed. See design.md
- [x] 3.5 Do not write `hire.jobFilters` — confirm the guess only reaches storage once the visitor edits the filters and the existing save path runs
- [x] 3.6 Confirm the branch is unreachable for the company-embedded list (the `standalone` guard already wrapping the restore)

## 4. Saying the scope was guessed

- [x] 4.1 Thread "this scope was inferred" from where it is applied to `HeaderLocationFilter.svelte`
- [x] 4.2 Mark the trigger accordingly and extend its accessible name to say so in words
- [x] 4.3 Drop the marking the moment the visitor edits or clears the scope
- [x] 4.4 Confirm a URL-seeded or restored scope is never marked

## 5. Page-experience cost

- [x] 5.1 Keep the outgoing rows on screen until the scoped list has painted. **Two bugs found by measuring, not by reading:** the release effect tracked its own write and cleared the hold in the same tick, and the empty-state branch read `jobs.items` directly and drew "No matching jobs" over a list that was merely reloading. Both fixed; every row read now goes through one `displayItems`
- [x] 5.2 Issue the region request as early as the precedence allows — first thing after the URL re-seed, and only for a browser that passes all three guards
- [x] 5.3 Measured with the country pinned by header, alternating pairs, against live prod results. **The figure is data-dependent, not a constant**: 0.026 in one session and 0.246 an hour later, from the same build — verified by stashing the later edits and re-measuring, which reproduced 0.246 exactly. Baseline 0.0014 in both. **LCP** showed no increase beyond noise either time. Before the hold-over was fixed the same measurement read 0.87
- [x] 5.4 **Decision: ship the automatic form**, taken with the range above in hand and not with a single flattering number. Holding the rows removes the collapse but not the height difference between the outgoing and incoming twenty, so the shift tracks whatever the catalogue happens to be serving. Accepted deliberately; the watchdog and CrUX are the checks, and the suggestion form is the recorded fallback
- [ ] 5.5 Confirm on prod that the scheduled Lighthouse watchdog still clears the floors in `perf/lighthouse/lighthouserc.json`, given it always takes the derived-scope branch

## 6. Verification

- [x] 6.1 Unit-tested the precedence in `shouldOfferGeoScope` — extracted from the component for exactly this reason, so each rule is a stated case rather than a branch inside a Svelte effect
- [x] 6.2 Unit-tested the clear-and-return case in both places it lives: the marker surviving `saveJobFilters('')` in filterStorage, and `offered: true` suppressing the guess in shouldOfferGeoScope
- [x] 6.3 Ran the feed against real prod data with `CF-IPCountry: BR`: URL becomes `?regions=latam,global`, the chip reads "LATAM +1" under a dashed underline, `hire.jobFilters` stays unwritten, and all five precedence cases behave (URL geography wins, any URL param suppresses, clean browser gets the guess, a cleared scope is never re-imposed, a stored set wins)
- [x] 6.4 Compared whole documents for BR and US, two fetches each. They differ in exactly two places, both country-independent and both also differing between two fetches from the SAME country: the per-request CSP nonce and a ticking "N seconds ago". Normalize those and all four hash identically
- [x] 6.5 Endpoint answers Googlebot and ClaudeBot with no region, a browser with its region, and the Lighthouse watchdog with its region — the watchdog must keep walking the visitor's path
- [x] 6.6 `pnpm vitest run` (1160 passed), `pnpm check` (0 errors), `pnpm run build`, eslint, and the design-system token ratchet all pass

## 7. Ops (separate repo: freehire-ops)

- [ ] 7.1 Read the Cloudflare zone's current ruleset before changing anything — a ruleset `PUT` replaces rather than merges
- [ ] 7.2 Turn on IP Geolocation for the zone
- [ ] 7.3 Forward `CF-IPCountry` from nginx to the SSR server
- [ ] 7.4 Verify on prod that the header reaches SSR, and that a request without it still serves the unfiltered feed
