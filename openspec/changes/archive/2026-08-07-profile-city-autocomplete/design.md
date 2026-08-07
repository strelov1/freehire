## Context

`internal/location/cities.go` already embeds a global, population-sorted GeoNames dataset
(`cities15000.tsv`, ~34k places) into `cityDict`, an **alias-keyed exact-lookup map** built
once at init: `lowercase-alias → {Name, Country}`. It backs `location.Parse` (job text) and
`location.ParseResidence` (candidate text), both of which resolve a whole free-text string to
zero or one place — there is no notion of "give me the top N places starting with this
fragment" anywhere in the package, because nothing before this change needed one.

The web app's `web/src/lib/components/facets/RemoteSearchSelect.svelte` already implements
exactly the client-side half of a debounced search-and-pick control (single call: `search:
(query) => Promise<FacetOption[]>`); it is imported into `ProfileForm.svelte` but currently
unused — the file wires its own plain `<Input>` for city instead. The company and subindustry
pickers show the established server-side half of the pattern: a small, unauthenticated,
GET-only handler under its own `internal/handler/<name>.go` file, registered directly on the
`api` router group, returning `{"data": [...]}`.

## Goals / Non-Goals

**Goals:**
- Let a profile's `base.city` and `relocation.cities` be picked from real places instead of
  typed blind, reusing the dictionary and UI pattern that already exist.
- Keep the `location_preferences` wire contract exactly as `search-profiles` already
  specifies it (city as free text) — this is a UI improvement, not a schema change.

**Non-Goals:**
- Building a prefix-index data structure (trie, sorted-string search, external search
  engine). At ~34k rows, a linear scan in Go is sub-millisecond; the existing `cities` job
  facet already tolerates a similarly-sized in-memory scan pattern, and `RemoteSearchSelect`
  debounces to one request per 250ms of typing, not one per keystroke.
- Changing what `location.Parse`/`ParseResidence` do, or the `cityDict` structure they use.
  The new search path reads the same source data but is additive.
- Disambiguating same-named cities across countries by storing a composite key. The stored
  value stays a bare city name, matching today's contract and today's ambiguity — the picker
  only makes the *choice* clearer (the label carries the country), not the *storage*.
- Any change to the base-country `<select>` beyond what the already-shipped `color-scheme`
  fix (separate, prior change) covers.

## Decisions

**A dedicated ordered slice, built once at init, alongside `cityDict`.** `cityDict` is a Go
map (unordered) keyed by alias, with many aliases collapsing onto one `cityEntry` — iterating
it at request time would require re-deduplicating and re-sorting by population on every
call. Instead, a sibling build step (`loadCitySearchIndex`) walks the same TSV once, in file
(population-sorted) order, producing one `citySearchEntry` **per row** (not per alias) —
each row is one real GeoNames place, so this is the only unit that can never
silently conflate two distinct places or lose one. `SearchCities` scans this slice, not the
`cityDict` map.

**An override renames a row only when its country matches AND no earlier row has already
claimed it.** `cityOverrides` (the existing `internal/location` table used by `cityDict`) is
alias-keyed, not row-keyed — nothing in it says *which* GeoNames row a given override was
curated for, only which alias string should resolve to it. A row's own alternate-name list
can legitimately, coincidentally contain that same alias string for an unrelated place: two
rounds of review surfaced two real instances — Frankfort, US shares the literal alternate
name "Frankfurt" with the `frankfurt` → Frankfurt-am-Main override (a different country, so
a same-country guard alone stops it), and Frankfurt (Oder), DE shares that *same* alias in
the *same* country as the true target, which a country guard cannot distinguish. The fix
that closes both: apply an override to a row only if (a) the override's target country
equals the row's own raw country, and (b) no earlier — i.e. more populous, since the file is
population-sorted — row has already claimed that exact override target. The larger, earlier
Frankfurt am Main claims the `frankfurt` override first; Frankfurt (Oder), reached later in
the scan, finds it already claimed and keeps its own raw name instead of being renamed and
then deduped away as a false duplicate.

**Matching: case-insensitive prefix on the canonical name, falling back to alias prefix.**
A user typing "Flor" should reach "Florianópolis" (canonical-name prefix); a user typing an
alias GeoNames or a curated override records (e.g. a native-language or ASCII-folded
spelling) should still reach the same entry. Each dictionary entry keeps its alias list
in scope for the fallback pass. Substring (not just prefix) matching was considered and
rejected for v1 — prefix matches what the profile's other search-select controls
(`SearchSelect`'s `optionMatches`) already do for a typed query, and keeps ranking simple:
first-N population-sorted prefix hits, no relevance scoring needed.

**Ranking: population order, already given by file order — no new ranking logic.** This is
the same "most-populous wins" discipline `cityDict`'s alias collapsing already uses
(`city-dictionary` capability); reusing file order for search ranking needs no new
comparator.

**Result cap: a fixed limit (20), matching `RemoteSearchSelect`'s existing max-height
scrollable list** — consistent with how the company/subindustry pickers already bound their
result lists; the control's own debounce and re-query-on-keystroke covers narrowing further.

**Endpoint shape and placement: `GET /geo/cities?q=&country=`, new
`internal/handler/geo.go`, public (no auth middleware), registered directly on the `api`
group** — mirrors `companiesHandlers`/`CompanySubindustries` exactly: a small struct + `register(api, mw)`,
unauthenticated because this serves the same public geography reference data the job-search
facets already expose, not anything user-scoped.

**Response shape: `{"value": "<name>", "country": "<iso code>"}` — the human-readable label
is composed client-side, not by the endpoint.** `value` is what gets saved into
`location_preferences`, unchanged from today's free-text shape (Decisions → Non-Goals:
storage stays a bare name). For the country name, `web/src/lib/facets.ts` already has
`countryLabel(code)` — an `Intl.DisplayNames`-backed resolver with no hand-maintained
table, used to build `COUNTRY_OPTIONS` for the very `<select>` next to this field. Having
the endpoint ship a raw ISO code and letting `searchCities()` compose
`` `${name}, ${countryLabel(country)}` `` reuses that resolver instead of standing up a
second country-name table in Go — the backend only needs the code it already has in
`cityEntry.Country`.

**Frontend: reuse `RemoteSearchSelect` for both fields; no new component.** For
`relocation.cities` (already a `string[]`) this is the component's native mode — `include =
relocCities`, `onToggle` = the existing `toggleIn` add/remove. For the single-valued
`base.city`, the parent constrains the component to at most one selection: `include =
baseCity ? [baseCity] : []`, and `onToggle` replaces rather than appends
(`baseCity = baseCity === value ? '' : value`). This was chosen over introducing a
single-select variant of the component because the existing multi-select rendering (a chip
above the input, cleared via its own ✕) already *is* the correct single-value UI once the
call site caps `include` at length 1 — no new component, no new prop.

**City search for `base.city` is narrowed by the already-chosen `baseCountry`; relocation
search is not.** `base.city` has an adjacent, always-visible country field to narrow
against; `relocation.cities` is a set with no per-item country, so narrowing by one country
would be wrong once more than one is intended. `SearchCities`'s `countryCode` parameter is
therefore optional and the relocation call site simply omits it.

## Risks / Trade-offs

- **[Same-named cities across countries are ambiguous without a country filter]** → The
  relocation-cities picker has no per-item country to narrow by. Mitigation: the label
  always names the country (`"Springfield, US"`), so the choice is visually disambiguated
  even though the stored value is the bare name — no worse than today's free text, which
  carried no country signal at all.
- **[An existing user's previously saved free-text city may not match any dictionary
  entry]** → `RemoteSearchSelect`'s existing `fallbackLabel` mechanism already renders a
  selected value verbatim when it's absent from the current result set; wiring
  `fallbackLabel={(v) => v}` means an unrecognized saved city still shows correctly instead
  of appearing to vanish.
- **[Country code mismatch between the web's `COUNTRY_OPTIONS` values and `cityEntry.Country`]**
  → Both are already lowercase ISO 3166-1 alpha-2 (`search-profiles` requires
  `base.country` as alpha-2; `city-dictionary` requires the same shape for its country
  field) — confirmed matching shape, no adapter needed.

## Migration Plan

None. Additive endpoint, additive dictionary-derived index built at process start (no
data migration), and a UI change with no wire-format change — a normal deploy; no backfill,
no flag, nothing to roll forward or back beyond the deploy itself.
