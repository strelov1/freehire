## Context

`freehire` surfaces curated collections on the `/collections` hub. Two kinds exist
in `web/src/lib/collections.ts`:

- `COLLECTIONS` — company-membership collections (`yc`, `bigtech`, …), mirroring the
  Go registry; each maps to the search facet `collections=<slug>`.
- `FILTER_COLLECTIONS` — frontend-only attribute collections, each mapping to an
  arbitrary set of `/jobs` facet params (currently just `remote-worldwide`).

Every hub card links to `/jobs?collections=<slug>` (or `/jobs?<params>`). The
`/jobs` list route hard-codes `canonical = <origin>/jobs`, so every filtered
variant — including collections — canonicalises to the bare list. The curated copy
never earns an indexable URL.

`JobsView` already supports a `scope` prop: a `Record<string, string>` of fixed
search params merged into every search but kept out of the URL and the
user-facing filter set. Company pages use it (`scope={{ company_slug }}`). This is
exactly the primitive a collection landing needs.

## Goals / Non-Goals

**Goals:**
- Each curated collection gets an indexable, self-canonical landing at
  `/collections/:slug` with its own `<title>`, `<h1>`, and description.
- Reuse existing filtering/feed machinery — no new pagination or facet code.
- Grow the attribute-collection set (regional remote landings) as pure data.
- Fix the missing `<h1>` on `/jobs` and `/companies` along the way.

**Non-Goals:**
- Redirecting the old `/jobs?collections=<slug>` URLs (they remain valid manual
  filters; canonical consolidation already handles the duplicate signal).
- Multi-value (`OR`-list) scope params — every current collection param is
  single-valued; a seam is left, not built.
- Backend/spec changes to the search API or the Go collections registry.
- A generated contract for `FILTER_COLLECTIONS` (still hand-kept; fold into
  gen-contracts only if the set outgrows a single file).

## Decisions

**Route `/collections/[slug]` reusing `JobsView` `scope`.** The `+page.server.ts`
resolves the slug against a unified lookup over both registries, builds the
collection's facet params, and fetches the SSR first page
(`searchJobs(params, 20, 0)`) — mirroring `/jobs/+page.server.ts`. The
`+page.svelte` renders `<Seo>` (self-canonical), a visible `<h1>` + intro copy,
then `<JobsView {initial} scope={params} excludeFacets={<collection facet keys>} />`.
This is the same shape as `/companies/[slug]` → `CompanyView` → `JobsView`.
_Alternative rejected:_ a bespoke read-only feed component — more code, loses the
built-in filter/pagination/count behaviour for free.

**Unified slug lookup.** A single exported helper resolves a slug to
`{ title, description, params }`, checking `FILTER_COLLECTIONS` (own `params`) then
`COLLECTIONS` (`{ collections: slug }`). Slugs are unique across both sets; the
helper is the single source for the route, the hub links, and the sitemap, so they
cannot drift. `scope` is `Record<string, string>`; the helper asserts every param
value is a single string (current data holds), leaving the array seam explicit.

**Template copy from the existing `title`.** `<title>` = `"<title> jobs · freehire"`,
`<h1>` = `"<title> jobs"`, description/intro = the existing `description`. No new
per-collection copy fields, no placeholders. An optional override field is a future
seam if a template ever reads awkwardly.

**Regional remote collections as data.** New `FILTER_COLLECTIONS` entries using
validated facet values — regions from `REGION_LABELS`
(`global, north_america, latam, eu, uk, mena, africa, apac, cis`) and countries as
ISO alpha-2. Note the vocabulary: there is no `us` region — Remote US and Remote
Brasil are country-level (`countries: us` / `countries: br`); Remote Latam is
region-level (`regions: latam`). Each new collection is shipped only after its live
count is confirmed healthy and non-empty.

**Sitemap via `STATIC_PATHS`.** Add `/collections` and `/for-companies` to
`STATIC_PATHS`, plus one `/collections/:slug` per entry in the unified registry.
The set is small and fixed (~a dozen), so it stays in the static-pages sub-sitemap —
no new sub-sitemap or backend query.

**List-page `<h1>`.** Add one semantic `<h1>` to `JobsView` and `CompaniesView`
(e.g. "Tech jobs" / "Companies hiring in tech"), styled to match the existing page
headers.

## Risks / Trade-offs

- **Thin/empty collection landings hurt SEO** → gate each new collection on a
  healthy live count before shipping; skip any that are near-empty.
- **Invalid facet values would render an empty feed** → params validated against
  the generated `REGION_LABELS` / country vocabulary, never guessed.
- **Duplicate signal between `/collections/:slug` and `/jobs?collections=<slug>`**
  → the collection landing is the canonical, internally linked (hub) and
  sitemapped home; `/jobs?collections=<slug>` canonicalises to `/jobs`, so it does
  not compete. Acceptable, no redirect needed.
- **`JobsView` embedded mode** (non-standalone) hides the swipe button and routes
  header search into the scoped list — identical to company pages, so the UX is
  consistent and already proven.

## Migration Plan

Pure additive frontend change; no schema or data migration. Deploy is a standard
`web` build. Rollback is reverting the route + data entries; old `/jobs?…` filter
URLs keep working throughout. No reindex needed (facet params already indexed).

## Open Questions

- Final curated list of regional remote collections to ship in this iteration —
  decided at implementation time from live counts (start set: Remote Latam, Remote
  Brasil, Remote US; extend to Remote Europe / APAC if counts warrant).
