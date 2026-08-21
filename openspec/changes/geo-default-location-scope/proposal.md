## Why

A first-time visitor lands on an unscoped catalogue: every job on earth, ordered by
freshness, most of them somewhere they cannot work. The geography they care about is
the one fact the site can already infer without asking — the request arrives through
Cloudflare, which names the visitor's country for free. Turning that into the opening
scope ("LATAM + Worldwide") replaces a global wall of postings with a plausible first
page, and costs the visitor one click to undo.

## What Changes

- The jobs feed adopts a **geography default derived from the visitor's IP country**
  when nothing else has said what geography to show: the country's region plus the
  worldwide region, applied as the `regions` facet.
- The country arrives on `CF-IPCountry`. Country → region reuses the existing
  `COUNTRY_REGION_MAP`; no new dictionary, no GeoIP database, no third-party lookup.
- The region is served from **its own endpoint, marked not to be stored**, and never
  travels inside a page's server-render payload — page data is embedded in the
  cached document, so carrying it there would make the HTML differ by country.
- The default is applied **after hydration, on the client**, never in the server
  render. The HTML a visitor receives does not vary by country, so the edge cache
  keeps one entry per URL and nothing needs a `Vary` header or a cache-key change.
- **Crawlers are never scoped.** Most traffic here is automated; a rendering crawler
  that got a region would index a feed scoped to its exit address.
- The default fires **at most once per browser**. A sticky marker records that the
  guess was offered, so a visitor who clears the scope is never re-scoped on their
  next visit.
- The chip states plainly that the scope was guessed, and clearing it is the same
  one click that clears any other scope.
- Scope is the standalone jobs feed only. `/companies` keeps its current behaviour —
  its geography answers "where is this employer", not "where may I work from", and
  the two are not interchangeable.
- No change to the catalogue API and no schema change. The one new route is the
  region endpoint above, which serves the browser and nothing else.

## Capabilities

### New Capabilities

- `geo-default-scope`: deriving an opening geography from the visitor's IP country,
  where it may and may not be applied, its precedence against every other source of
  filter state, and the one-shot rule that keeps it from overriding a deliberate
  clear.

### Modified Capabilities

- `filter-persistence`: the restore chain gains a lowest-priority source. Today a bare
  `/jobs` with no URL params and no stored filters shows the unfiltered list; it may
  now show the geo default instead. The existing precedence (URL params beat storage)
  is unchanged and the geo default sits below both. The requirement that a cold load
  performs no restore needs its boundary restated: the geo default is not a restore of
  a stored filter set and does apply on a cold load.
- `header-location-filter`: the trigger must be able to say that the current scope was
  inferred rather than chosen, so a visitor who did not pick "LATAM" is not left
  guessing why the catalogue looks small.

## Impact

- A new `+server.ts` endpoint — reads `CF-IPCountry`, answers `{ region }`
  uncached, and answers crawlers with nothing.
- `web/src/lib/filters.ts` / the standalone `/jobs` seeding path — the new lowest-priority
  source, and the one-shot marker beside `hire.jobFilters`.
- `web/src/lib/components/HeaderLocationFilter.svelte` — the "guessed" affordance.
- **freehire-ops (separate repo)** — nginx must forward `CF-IPCountry` to the SSR
  server, and the Cloudflare zone's IP Geolocation toggle must be on. Without either,
  the header is absent and the feature is inert by design.
- Behaviour when the header is missing (direct origin hit, local dev, geolocation off)
  is the current behaviour: no default, unfiltered list.
