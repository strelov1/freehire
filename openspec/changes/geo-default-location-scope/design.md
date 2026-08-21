## Context

The standalone jobs feed seeds its filters from the URL, and `JobsView.svelte`
restores a stored filter set (`hire.jobFilters`) when a client-side navigation lands
on a bare `/jobs`. That restore deliberately skips the router's initial `enter`
navigation — a hard load, refresh, or direct URL — because `replaceState` is not
safe before the router is initialized, and because the server already rendered that
exact URL.

The change adds one more source of opening geography, below both, derived from the
visitor's IP country. Two constraints shape every decision below:

- **The feed is edge-cached by URL.** Anything that makes the server response depend
  on the visitor's country multiplies the cache and risks serving one country's page
  to another. This project has already been bitten by a stale body pinned into
  Cloudflare, so the safe answer is not "tune the cache key" but "do not vary".
- **The derived scope must apply exactly where the existing restore does not** — on
  a cold load. That is the one navigation type the current mechanism opts out of.

Cloudflare supplies `CF-IPCountry` free on every plan when the zone's IP Geolocation
toggle is on. The country→region grouping already exists as `COUNTRY_REGION_MAP`,
generated from `internal/location` into `web/src/lib/generated/contracts.ts`.

## Goals / Non-Goals

**Goals:**

- A first-time visitor's opening jobs feed is scoped to their region plus worldwide.
- The server-rendered response is byte-identical regardless of the visitor's country,
  including its embedded data payload.
- Automated clients are never scoped.
- A visitor who clears the guessed scope is never re-scoped, on any later visit.
- The feature is inert, with no error and no behaviour change, wherever the header
  is absent: local dev, a direct origin hit, a zone with geolocation off.

**Non-Goals:**

- City- or country-level precision. The scope is a macro-region; the header's country
  is used only to pick one.
- Any geolocation source beyond the edge header — no GeoIP database, no third-party
  lookup, no browser geolocation prompt.
- Scoping `/companies` or the company-embedded jobs list.
- Persisting the guess as if it were a chosen filter set.
- Server-side application, redirects, or a cache-key change.

## Decisions

### Serve the region from its own uncached endpoint, never through page data

A dedicated `+server.ts` route reads `CF-IPCountry`, normalizes it, maps it to a
region, and answers `{ region }` with `Cache-Control: private, no-store`. The client
calls it only when the guess is actually in play — marker unset, no geography in the
URL, no stored set to restore — so it costs one small request on a genuine first
visit and nothing at all on every subsequent one.

*Alternative considered — put the region on `event.locals` and return it from the
root `+layout.server.ts`, the way `locale` is handled.* **Rejected, and it was the
first design.** Server-load data is serialized into the document SvelteKit ships, so
a region delivered that way makes the HTML differ by country — the exact failure the
whole client-side approach exists to prevent. `locale` gets away with it because it
comes from a cookie the visitor's own request carries, not from a value the edge
derives per request. The distinction is easy to miss and worth stating: "computed on
the server" and "embedded in the cached response" are the same thing here.

*Alternative considered — set the region as a cookie at the edge and read it in the
browser.* Workable, and rejected for this codebase: it puts a value the app depends
on outside the app, in a Cloudflare rule that no test covers and no deploy of this
repo touches. The endpoint keeps the derivation next to the code that consumes it.

### Crawlers get no region

The endpoint answers a recognized crawler user agent with no region. Most of this
site's traffic is automated, and a rendering crawler that received one would index a
feed scoped to wherever its exit address happens to sit — a scope no canonical URL
describes. Non-rendering crawlers never reach the endpoint at all, so the check only
has to catch the ones that run JavaScript.

This lives at the endpoint rather than in the client because the client cannot know
what it is, and because a check the client could skip is not a check.

### Normalize at the boundary: `XX`, `T1`, and unknown codes become "no region"

Cloudflare sends `XX` when it cannot place an address and `T1` for Tor exits. Both
are valid-looking two-letter codes that are not countries. They are filtered where
the header is read, along with any code the country→region grouping does not carry,
so nothing downstream has to know they exist. The result handed to the client is
either a region value or nothing — never a country code needing interpretation.

### Apply the scope on the client, in the same `afterNavigate` that restores storage

Putting the derived scope beside the existing restore keeps one place that decides
what a bare `/jobs` opens with, and makes the precedence readable as a single
ordered expression rather than two mechanisms that have to agree:

1. URL has filter params → seed from the URL, nothing else runs.
2. Client-side navigation with an empty search and a stored set → restore it.
3. Otherwise, if the marker is unset and a region was derived → apply the derived
   scope, set the marker.
4. Otherwise → the unfiltered list, as today.

*Alternative considered — a separate `$effect` or `onMount` hook.* Rejected: it
would race the restore, and the two would need a shared flag to avoid both writing
the URL. The precedence is the feature; it belongs in one expression.

### Write the URL directly and re-seed from it, rather than through `filters.apply()`

**Revised during implementation.** The plan said `goto(…, { replaceState: true })`,
on the reasoning that shallow `replaceState` is unavailable during the initial
`enter`. Two things turned out to matter more.

First, `filters.apply()` — the obvious way to set a filter set — is an *explicit
write*, and on the standalone list an explicit write mirrors itself into
`hire.jobFilters` through the store's persist callback. The spec forbids persisting
the guess, so `apply()` was never an option regardless of how the URL got written.

Second, the `enter` objection expires. The guess is applied *after* awaiting a
network round trip, by which point the router is long initialized. So the write is a
plain shallow `replaceState` followed by `filters.syncFromUrl()` — the same path an
ordinary navigation re-seed takes, which by construction fires no persist callback
and needs no new store API. It also avoids re-running the route's `load`, which
`goto` would have done for a page already in hand.

This is the one place the derived scope costs the visitor something real: the server
rendered an unscoped first page, and the client immediately replaces it with a scoped
one. That flash is the accepted price of not varying the cached document — chosen
deliberately over the alternatives below.

*Alternative considered — a server-side redirect for country-scoped visitors.*
Rejected: the redirect response itself varies by country, so the cache problem moves
rather than disappears.

*Alternative considered — render the feed unscoped and offer the scope as a
dismissible suggestion.* A real option with no flash, and weaker: most visitors will
not click it, so the opening catalogue stays a global wall for most of them. Recorded
here because if the flash proves worse in practice than it reads on paper, this is the
fallback that needs no new infrastructure.

### Apply the region **and** worldwide, rather than the region alone

A regional scope that excludes worldwide-remote postings hides the part of this
catalogue a visitor is most likely to want: the roles open to them precisely because
they are open to everyone. Both go in together.

*Alternative considered — apply the narrow scope and offer the broader one as a
one-click suggestion in the filter popover.* This is the shape another job search
product ships, and it guesses more conservatively. Rejected here because it is a
second feature — a suggestion card with its own placement, copy, and dismissal — and
because their narrow default is a country while ours is already a macro-region, so
the gap it leaves is smaller and worth less than the UI to close it.

### Hold the list's height across the swap, and let the watchdog judge the rest

The swap is the one part of this change that a search engine measures. Two effects,
both in the window before any user input:

- **Layout shift.** The scoped list has a different number of rows and a different
  height. Left alone, everything below it moves. Mitigation: the list container keeps
  the height it rendered with until the replacement has painted, so the swap happens
  inside a box that does not resize.
- **Largest contentful paint.** A later, larger paint replaces the earlier candidate.
  Mitigation is only partial — start the region request as early as the precedence
  allows, so the swap lands close to the original paint rather than hundreds of
  milliseconds after it.

The second one cannot be argued away, only measured. `perf/lighthouse/lighthouserc.json`
already runs the watchdog against the feed on a schedule, and it does so with empty
browser storage — meaning it will take the derived-scope branch on every run, and
becomes the guard for this change for free. Two consequences worth stating before
someone debugs them in the dark:

- The watchdog's region depends on where its runner sits, so the scope it measures is
  not fixed run to run. That is acceptable for a score floor and useless for a
  before/after comparison; pin the country with a header for the comparison run.
- If the floors start failing after this ships, the derived scope is the first
  suspect, not the last.

*Alternative considered — exempt the watchdog.* Rejected outright. A guard that does
not measure the path real visitors take is worse than no guard, because it reports
health that nobody has.

**If measurement says this costs more than it buys**, the suggestion form below is not
a consolation prize, it is the answer: no swap, no shift, no late paint, and the
feature survives in a shape that cannot regress the metrics at all.

### A marker key separate from `hire.jobFilters`

`saveJobFilters('')` removes the key, so "storage is empty" cannot distinguish a
browser that has never filtered from one that just cleared its filters. Keying the
guess on that would undo a deliberate clear on every subsequent visit. A separate key
records the offer itself, is written when the scope is applied, and is never removed
by clearing filters.

It lives in `filterStorage.ts` next to the existing key, wrapped the same way: every
access feature-detects `localStorage` and swallows failures.

### Storage unavailable means the guess does not run

When storage throws, the marker can be neither read nor written, so the guess would
re-apply on every single page load and fight anyone who cleared it. The safe reading
of an unreadable marker is "already offered". The feature turning itself off in
private mode is a smaller loss than a scope that cannot be dismissed.

## Risks / Trade-offs

- **A visible content swap on first load** → **Measured, and the cost turned out to
  be a range rather than a number.** With the outgoing rows held on screen the same
  build read CLS 0.026 in one session and 0.246 an hour later, against a 0.0014
  baseline both times; LCP was unmoved in both. Stashing the later edits and
  re-measuring reproduced 0.246 exactly, which rules out the code and leaves the
  data: the hold removes the collapse but not the height difference between the
  outgoing twenty rows and the incoming twenty, and that difference is whatever the
  catalogue is serving that hour. Shipped automatic anyway, deliberately and with
  those figures in hand; the watchdog and CrUX are the checks that can still overturn
  it, and the suggestion form is what it would be overturned in favour of.

  Two things about how that number was reached are worth keeping, because both
  produced a confident wrong answer first:

  - The first measurement ran against a built `adapter-node` server, which does NOT
    proxy `/api` — only the Vite dev server does. `API_INTERNAL_URL` fixes the server
    render and nothing else, so the SSR list arrived and the client's reload 404'd.
    CLS read 0.50, and it was measuring an error message replacing a list.
  - The second ran on a working harness and read 0.87, because the hold-over was
    broken twice over: its release effect tracked its own write (clearing the rows in
    the same tick that captured them), and the empty-state branch read the paginator's
    items directly, drawing "No matching jobs" over a list that was only reloading.
    Fixing both took the same measurement to 0.026.

  The tell in both cases was the shift attribution: a `footer` going from height 0 to
  386 is a page collapsing, which no ordinary list swap does.
- **The watchdog now measures a path that varies by runner location** → Its score
  floor still works, since every region produces a comparable page. A before/after
  comparison needs the country pinned by header, or it compares two different pages.
- **The scope hides jobs the visitor might want** → Mitigated by always including the
  worldwide region, so remote-anywhere roles are never filtered out, and by the chip
  saying the scope was inferred with a one-click clear.
- **A VPN or a travelling visitor gets the wrong region** → Mitigated by the same
  one-click clear, and by the once-per-browser rule: a wrong guess is corrected once,
  not re-imposed on every visit.
- **The ops half is in another repo** → The nginx forward and the Cloudflare toggle
  live in `freehire-ops`. If either is missing the header never arrives and the
  feature is silently inert — which is the correct failure, but it will look like the
  code is broken. Verify the header reaches the SSR server before concluding anything
  about the client code.
- **Country → region is a product judgement, not a fact** → The grouping is the one
  already used everywhere else in the catalogue. Any disagreement with it is a
  disagreement with the existing region taxonomy and should be settled there, not
  worked around here.

## Migration Plan

1. Ship the client and server code first. With no header arriving, it is inert — no
   flag needed, because the header's absence is the off switch.
2. Turn on IP Geolocation in the Cloudflare zone and forward `CF-IPCountry` in
   `freehire-ops`. Read the zone before applying: a ruleset `PUT` replaces rather
   than merges.
3. Verify on prod with a request that carries the header and one that does not.
4. Rollback is stopping the header at nginx — no deploy, no revert.

## Open Questions

- How the chip marks an inferred scope visually is left to implementation; the spec
  requires only that it is marked and that the accessible name says so in words.
- Whether the derived scope should be recorded as an analytics event (offered / kept
  / cleared) to measure whether it helps. Cheap to add, and the only way to learn
  whether the flash buys anything.
