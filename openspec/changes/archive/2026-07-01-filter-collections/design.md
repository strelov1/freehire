## Context

The `/collections` hub currently renders one card per entry in the company-collection
registry (Go `internal/collections`, mirrored for display in `web/src/lib/collections.ts`).
Membership is a company-level fact denormalized to the `jobs.collections` search
facet; a card links to `/jobs?collections=<slug>` and its count reads from the
`collections` facet distribution (a single facet call in `+page.server.ts`).

We want the hub to also offer curated groupings defined by an arbitrary `/jobs`
facet filter (the first being "Remote Worldwide" = `work_mode=remote` +
`regions=global`). Such a grouping is a property of a job, not a company, so it
cannot reuse the company-membership machinery. But the `/jobs` feed already filters
on these params, and `searchJobs(params, 0, 0)` already returns a `meta.total` — so
a filter collection needs no backend at all: it is just curated presentation over
the existing search.

## Goals / Non-Goals

**Goals:**
- A general mechanism: any browsable `/jobs` filter can become a collection card
  with a one-line data entry.
- Filter collections appear on `/collections` as cards visually identical to
  company-collection cards.
- Zero backend surface: no Go, no DB, no migration, no reindex, no API change.

**Non-Goals:**
- No dedicated per-collection landing page (parity with company collections; the
  `/jobs` feed is the single rendering).
- No exposure of filter collections through the public API or notifications /
  saved-searches (deferred; frontend-only for now).
- No visual distinction (badge/section) between the two kinds in v1 (YAGNI).

## Decisions

**1. Filter collections are a separate frontend type, not an overload of the
company-collection type.** They genuinely differ (no company membership, no facet
value, no Go). A sibling `FilterCollection { slug, title, description, params }`
array in `web/src/lib/collections.ts` keeps each concept honest, versus adding an
optional `params` to the existing `Collection` and branching on its presence.

**2. `params` is a structured `Record<string, string | string[]>`, not a raw query
string.** The object is the single source for both the card `href` and the count
request, and it is type-checked. A raw string would have to be parsed in both
directions and a typo (unknown facet key) would silently yield an empty filter —
the search layer ignores unknown params, so the bug would be invisible. A shared
`toQuery(params)` helper expands list values into repeated keys (OR semantics),
matching the `/jobs` filter contract.

**3. The server load normalizes both kinds into one card view-model
`{ slug, title, description, href, count }`.** The page component iterates a single
`cards` list and stays ignorant of the two kinds — company cards get
`href=/jobs?collections=<slug>` + facet-distribution count (unchanged), filter cards
get `href=/jobs?<query>` + a per-collection `searchJobs(toQuery(params), 0, 0).total`.
Filter counts run under `Promise.all` with per-collection try/catch; like the
existing facet count they are decorative and degrade to no count.

**4. Filter collections render first** in the grid — they are the broad
"browse by attribute" entries. Ordering is just array/concat order and trivially
adjustable.

Alternatives considered: (a) a full Go "filter-collection" registry exposed via API
— rejected as unnecessary coupling for a presentation concern (YAGNI); (b) two
separate grids/sections on the hub — rejected for v1 as visual noise, but the
unified card model leaves the door open.

## Risks / Trade-offs

- [One extra search request per filter collection on hub load] → Only a handful of
  filter collections exist; requests run in parallel (`Promise.all`) and each is
  `limit=0` (count only). Counts are decorative, so a slow/failed call degrades to
  no count without failing the page.
- [`web/src/lib/collections.ts` mirrors the Go company registry by hand] → We are
  adding a wholly frontend concept beside it, not touching that mirror; the existing
  drift caveat (fold into generated contracts if it grows) is unchanged.
- [A filter collection with a mistyped facet key would show an empty feed] →
  Mitigated by the typed `params` and by `svelte-check`; the seed uses known keys
  (`work_mode`, `regions`) verified against `search.StringFacets`.

## Migration Plan

Frontend-only, additive. Deploy the `web/` build; no data migration, no reindex.
Rollback is reverting the three touched files — company collections are untouched
throughout, so there is no state to unwind.

## Open Questions

None.
