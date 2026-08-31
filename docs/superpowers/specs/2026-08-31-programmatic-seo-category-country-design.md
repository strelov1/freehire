# Programmatic SEO: category × country landing pages

Date: 2026-08-31
Status: design approved, not implemented

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

```
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

One trap: `countryLabel` echoes the UPPERCASED code back when `Intl` does not know it
(the user-assigned `xk` returns `"XK"`), where the `seo.ts` copy returns `undefined`
for that case. An echo is not a name, so the index must drop any code whose label
equals its own uppercase form; otherwise `xk` would mint the URL `/roles/backend/xk`,
which is not a phrase anyone searches.

A pure function, covered by a test asserting the map is injective, that known codes
round-trip, and that an echoed code is absent. No new country table enters the
repository, so there is nothing to drift.

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

| Block | Source | Cost |
|---|---|---|
| H1 + auto-intro | derived from the figures below | 0 |
| Salary p25/p50/p75 + sample size | `GET /insights/salary?category&country` | 1 call |
| Top skills, seniority mix, work mode, visa, company size | `GET /jobs/facets?category&countries` | 1 call |
| Job list | `JobsView` with `scope` | 1 call |
| Neighbours: same category in 8 countries, 8 categories in this country | the facet call above | 0 |

Three upstream calls per SSR render, matching what `/insights/[category]` already
costs. `Cache-Control: public, s-maxage=3600`, as those pages use.

One `GET /jobs/facets?category=X&countries=Y` returns EVERY axis at once — skills,
seniority, work_mode, visa_sponsorship, english_level, reality, company_size. The bulk
of the differentiating content is one request, not one request per block.

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

Expected yield: ~2,000–2,200 pages of the 37 × 239 = 8,843 possible. Measured basis —
for three sampled categories, 56–62 countries each clear 50 postings.

## Scope of change

**The backend is not touched.** Both endpoints exist and answer today; verified
against production during design. No migration, no new query, no worker.

New work, all in `web/`:

- `web/src/routes/roles/` — three route levels, each with `+page.server.ts` (SSR load)
  and `+page.svelte`.
- `web/src/lib/roleLandings.ts` — the country-slug index, the publication gate, the
  deterministic intro composer, the neighbour selection. Pure functions, unit-tested,
  no I/O.
- `web/src/routes/sitemap-landings.xml/+server.ts`, registered in the sitemap index at
  `web/src/routes/sitemap.xml/+server.ts`. ~2.2k URLs fits one file (the format's cap
  is 50k), so no offset pagination is needed at this size.
- An nginx location for `/roles`, since the route lives outside `/api/` and the ops
  config does not route unknown top-level paths by default.

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

- Unit: country slug index is injective; known codes round-trip; unknown codes drop.
- Unit: the publication gate admits ≥50 and rejects below, off fixture facet counts.
- Unit: honesty rules — english block hidden under 20% coverage, salary block hidden
  under sample 10, reality rendered in the positive framing.
- Unit: the intro composer is deterministic for fixed input.
- Route: a below-threshold pair 404s; an above-threshold pair renders and
  self-canonicalises.
- Sitemap: only published pairs appear.

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
