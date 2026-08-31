# Programmatic SEO: category × country landing pages

Date: 2026-08-31
Status: implemented. The sections below are the design as approved, amended where
building it proved a number wrong — each amendment says so and gives the measurement.

## Problem

The catalogue holds ~1.35M open postings across 37 categories and 239 countries, but
nothing on the site targets the search pattern that carries most job-seeker intent:
`"<category> jobs in <country>"`. The two existing landing surfaces each cover one
axis only:

- `/collections/[slug]` — 92 hand-curated landings, each pinning a SINGLE facet
  (skill, role, region, seniority, or category). Adding one is a data entry in
  `web/src/lib/collections.ts`, so the set cannot grow past what a person maintains.
- `/insights/{salary,skills,roles}/[category]` — 25 categories × 3 page kinds, all
  global. No geography.

No page combines two axes. A search for "backend jobs in germany" finds nothing here
that ranks.

## Decision

Generate landing pages over the **category × country** product, served from a new
`/roles/**` route tree.

Category, not role, is the first axis. `role` looked like the richer choice (349
values clearing 300 open jobs, versus 37 categories) and was rejected for two reasons
found during design:

1. **Salary rollups are keyed by category.** `GET /api/v1/insights/salary` accepts
   `category`, `seniority`, and `country` — there is no `role` parameter, and the
   rollup behind it has no role dimension. A role-keyed page would have to borrow its
   parent category's band and caveat it. Category-keyed pages state a figure that is
   actually the figure.
2. **The `role` facet is not a closed vocabulary.** Its top values include `senior`,
   `management`, and `sales` — seniority and category values that leak into the same
   axis. A generator over it would mint URLs like `/roles/senior/germany`. `category`
   comes from the `internal/dict/classify` vocabulary, which is closed and validated.

A programmatic generator must be driven by a closed vocabulary: de-indexing pages that
should never have existed is far more expensive than not minting them.

Roles remain a candidate for phase 2, gated on what the category pages actually earn.

## URL structure and hierarchy

```text
/roles                         hub: all published categories
/roles/[category]              per-category country table
/roles/[category]/[country]    the landing: figures + job list
```

Subfolders, not subdomains, so authority accrues to the apex domain.

`[category]` is the dict slug with underscores rendered as hyphens
(`data_analytics` → `data-analytics`). `[country]` is the slugified English country
name (`de` → `germany`), never the ISO code — the code is not what anyone searches.

### Country slug resolution

`countryLabel` (exported from `web/src/lib/facets.ts:144`) already maps ISO alpha-2 →
English name via `Intl.DisplayNames`, deliberately holding no country table of its
own. It is the resolver the rest of `web/` uses — the filter chips, the company
country select, the city labels. A second private copy exists at
`web/src/lib/seo.ts:176` (`countryName`); the new code uses the exported one and does
not add a third.

The reverse direction (`germany` → `de`) is not something `Intl` offers, so it is
derived: iterate `ISO_COUNTRY_CODES` (`web/src/lib/facets.ts:488`), run each through
`countryLabel`, slugify, and index. The index is built once per process and belongs
beside `countryLabel` in `facets.ts` — the code list it reads is module-private and
should stay that way rather than be exported to a consumer.

One trap: `countryLabel` echoes the UPPERCASED code back when `Intl` does not know it,
where the `seo.ts` copy returns `undefined`. An echo is not a name, so the index drops
any code whose label equals its own uppercase form — otherwise an unknown code would
mint a URL out of a two-letter non-word.

How many codes that guard catches DEPENDS ON THE BUILD, which is why the test asserts
the invariant and not any one code: a full-ICU Node resolves all 249 (`xk` comes back
as "Kosovo", so the comment in `seo.ts` naming it as the unresolvable example is now
stale), while a `small-icu` build resolves few. The test therefore asserts that no
published slug is merely an ISO code, which holds under both.

Collisions are handled by dropping the WHOLE group, not by letting the first code win:
handing a slug to whichever came first in the list is an arbitrary choice made
silently, and on an indexed URL that means one country quietly serving another's page.
An absence is visible; a substitution is not. Measured: 249 countries resolve, zero
collide.

A pure function, covered by tests asserting the map is injective, that every published
code round-trips, and that slugs are URL-safe. No new country table enters the
repository, so there is nothing to drift.

### Case, and why it redirects

Both resolvers accept any case, which on its own would publish a page TWICE: a request
for `/roles/Backend/Germany` resolves, renders, and then builds its canonical URL,
breadcrumbs and sibling links out of the raw route params — so the self-canonical would
point at the mixed-case URL rather than at the one the sitemap lists.

Any spelling but the canonical one therefore 308s to it, and the loaders return the
canonical slugs rather than `params`. 308 rather than 404 because the request did name
a real page (`/collections` 404s here, but its lookup is case-SENSITIVE, so the case
never arises), and rather than merely fixing the canonical tag because one URL beats
two plus a pointer between them.

Found in review, not in design.

### Cannibalisation guard

15 categories ALREADY have a `/collections/<category>` landing, and those pages are
job *feeds*. `/roles/[category]` must therefore NOT be another feed, or the two
compete for the same query.

The split by content type:

- `/collections/backend` — the feed. "Show me backend jobs."
- `/roles/backend` — a country table with counts and median pay per country.
  "Where are backend jobs, and what do they pay?"

Different intent, different content, mutual links: `/roles/backend` links to
`/collections/backend` as "browse all backend jobs", and the collection links back as
"backend jobs by country". Both stay self-canonical.

The remaining 22 categories have no collection page; nothing to deconflict.

## Page composition (third level)

| Block | Source | Scope |
|---|---|---|
| H1 + auto-intro | derived from the figures below | — |
| Salary p25/p50/p75 + sample size | `GET /insights/salary?category&country` | the pair |
| Skills, seniority, work mode, visa, company size, english, reality | `GET /jobs/facets?category&countries` | the pair |
| Publication gate AND "same category, other countries" | `GET /jobs/facets?category` | the category |
| "Other roles hiring here" | `GET /jobs/facets?countries` | the country |
| Job list | `JobsView` with `scope` | the pair |

**Five upstream reads per render, all issued in parallel.** The design started at three
and grew by two once the scopes were worked out; the count is recorded here because it
is the page's cost, not a detail:

- The gate and the sibling-country links read the SAME country distribution under the
  category, so they are one call rather than two. The gate must read that call and not
  the pair call's own `total`, because the pair call carries the visitor's in-URL
  filters — a published page would otherwise 404 for anyone arriving with a filter
  narrow enough to push the filtered count under the floor.
- "Other roles hiring here" needs the country-scoped category distribution. The global
  one is free (it is what the hub reads) and would recommend management, sales and
  support on every page regardless of the country being viewed — a navigation block
  that does not describe where the visitor is standing.

One `GET /jobs/facets?category=X&countries=Y` returns EVERY pair axis at once, and the
call names exactly the ones rendered: counting is paid per facet, and the wide-valued
ones (cities, companies) would otherwise be counted for nothing.

`Cache-Control: public, max-age=0, s-maxage=3600` on all three levels, as `/insights`
uses — a crawler burst must not become one fan-out per request.

The intro paragraph is composed deterministically from the retrieved numbers, in the
manner of `web/src/lib/insights.ts`. No LLM: a generated sentence that cannot be
reproduced from the data is a sentence nobody can verify when it is wrong.

### Honesty rules

Measured against live production data during design, three blocks would have
misrepresented the catalogue if rendered unconditionally:

- **English level** is sparsely populated. In `backend × de`, `english_level` covers
  136 of 2041 postings. Rendering "C1 required in 90%" off a 6.7% sample states a
  property of the annotation, not of the market. **Render only when coverage ≥20% of
  the pair's total**, and label it as a share of postings that declare a level.
- **Reality** reads `stale: 1568, fresh: 470` for that same pair. "77% of postings are
  stale" is a true sentence that reads as an indictment of the catalogue rather than
  of the market. Render the positive framing — "470 added recently" — which carries
  the same signal and is the number a job seeker acts on.
- **Salary** always renders its sample size beside the figure ("median €95,000, from
  21 postings"). **Hidden entirely when the sample is under 10.** `backend × de`
  returns a 21-posting sample; `data_analytics × us` returns 3197. A page that shows
  both formats identically would imply an equal confidence that does not exist.

Currencies are never combined: the endpoint returns one row per (currency, period) and
each is rendered as its own line. `backend × pl` returns four such rows.

## Publication threshold

A (category, country) pair is published when it holds **≥50 open postings**. Below
that, the route returns 404 and the pair is absent from the sitemap.

This mirrors the gate `/insights` already runs (`MIN_CATEGORY_OPEN=25` in
`web/src/lib/insights.ts`), raised because a two-axis page splits its evidence across
more blocks than a one-axis page does.

The threshold is evaluated against the live facet distribution, so it self-heals in
both directions: a country that grows past 50 postings gains its page on the next
sitemap render without a deploy, and one that shrinks below loses it.

**Measured yield: 1,508 URLs — 37 category tables and 1,471 pairs**, of the 37 × 239 =
8,843 possible. Counted by walking every sub-sitemap against production once the
routes were built.

The estimate written during design was 2,000–2,200, extrapolated from three large
categories where 56–62 countries each clear the floor. It was high because the tail is
much thinner than the head: `management` publishes 88 countries and `sales` 86, while
`blockchain` and `developer_relations` publish ONE each (the US). Nothing is wrong with
a one-row table — the pairs behind it clear the same floor as every other — but the
distribution is worth knowing before anyone reads a traffic projection off the page
count.

## Scope of change

**The backend is not touched.** Both endpoints exist and answer today; verified
against production during design. No migration, no new query, no worker.

New work, all in `web/`:

- `web/src/routes/roles/` — three route levels, each with `+page.server.ts` (SSR load)
  and `+page.svelte`.
- `web/src/lib/roleLandings.ts` — the country-slug index, the publication gate, the
  deterministic intro composer, the neighbour selection. Pure functions, unit-tested,
  no I/O.
- `web/src/routes/sitemap-roles.xml/+server.ts`, sharded by category and registered in
  the sitemap index once per category.

  The whole product fits one sitemap file comfortably (1,508 URLs against the format's
  50k cap), but building it in one request would cost one facet call PER CATEGORY
  inside that request — 37 upstream reads to answer one crawler. Sharding by the axis
  the call is already scoped to makes each shard exactly one call, and the index can
  name all 37 shards without reading anything, since the category list is a
  compile-time constant. The hub `/roles` rides in `STATIC_PATHS` instead: it is the
  one page here that is not gated on live counts.
**No ops change.** The design assumed `/roles` would need an nginx location because it
sits outside `/api/`; it does not. `web/nginx.conf` ends in a `location /` catch-all
that proxies everything unmatched to the adapter-node SSR server, so a new top-level
route is served the moment it is built. Deploy is a plain `release.sh` — web build
only, no migration, no worker.

### Structured data

`BreadcrumbList` + `ItemList` per landing, emitted through the existing `$lib/seo`
helpers.

`JobPosting` markup is deliberately NOT emitted here. Each posting already carries it
on its own `/jobs/[slug]` page, and repeating it on an aggregate page duplicates the
entity across two URLs.

### Canonicals

Each landing is self-canonical. A landing reached with additional filter parameters
(e.g. `?seniority=senior`) canonicalises to the bare landing, matching the pattern
`/collections/[slug]` already follows: the SSR load threads `url.searchParams` and
then sets `scope` on top, so a shared filtered URL still renders filtered while
pointing search engines at the one canonical page.

## Testing

Unit (27 cases across `facets.test.ts`, `roleLandings.test.ts`, `sitemap.test.ts`):

- country slug index is injective, round-trips every published code, emits URL-safe
  slugs, and publishes no slug that is merely an ISO code;
- the category axis round-trips and excludes the `other` catch-all;
- the gate admits at exactly 50, rejects below, and drops a code carrying no slug
  however many jobs it holds;
- honesty rules — english null under 20% coverage and on an empty distribution, salary
  bands dropped under sample 10, currencies never merged, reality read as `fresh`;
- the intro is deterministic, omits the freshness clause at zero rather than writing
  it, and survives a pair with no skills annotated;
- `roleLandingPaths` adds nothing to the countries it is handed, which is what keeps
  the sitemap and the route's 404 reading off ONE rule.

The route level is verified by running the built server against the production API
rather than by a route test, since what is being checked is the wiring to real data:
`/roles`, `/roles/backend` and two pairs return 200; an unknown category, an unknown
country, the `other` catch-all and a slug-less code return 404; the sitemap index
lists 37 shards and a shard for an unknown category 404s. The rendered
`backend × Germany` page states 648 open jobs, 177 recent, a €95,000 median from 21
postings, visa sponsorship in 64% of the 77 postings that state a position — and
OMITS the english block, which is the honesty rule firing on live data.

Not covered by a test: that the pair page and `/collections/<category>` stay
different in content. Nothing enforces it; the note under "Cannibalisation guard" is
the whole defence, so a future change that turns `/roles/[category]` into a feed
would reintroduce the conflict silently.

## Deferred, deliberately

- **Roles as a third axis** (`/roles/[category]/[country]` → per-role variants). Gated
  on measured performance of the category pages, and on salary gaining a role
  dimension.
- **Cities.** ~554 cities clear 300 postings, which would multiply the page count by
  roughly 20. The crawl budget question has to be answered by observation first:
  declared bots already generate 73% of production requests, and every one of these
  pages is a full SSR render on a single-process web tier.
- **Batching the `/roles` hub's counts.** The hub needs a count per category; the
  known follow-up on the `/collections` hub is that it fires one uncached call per
  entry. The hub here reads all 37 counts off ONE unfiltered facet call, so the
  problem is avoided rather than deferred — noted so it is not reintroduced.
