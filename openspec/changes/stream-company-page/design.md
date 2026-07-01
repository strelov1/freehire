## Context

`web/` is a SvelteKit SSR app. The `/companies/[slug]` route has a
`+page.server.ts` whose `load` does:

```ts
const [{ company }, initial] = await Promise.all([
  client.getCompany(params.slug, 1, 0),
  client.searchJobs(facets, LIMIT, 0),
]);
return { company, initial, slug };
```

On a client-side navigation from `/companies`, SvelteKit fetches this route's
data (`__data.json`) and keeps the old page visible until `load` **returns**.
Because `load` `await`s both requests, the transition is blocked on the slower
one. Measured server-side TTFB (from a public network, so noisy):
`searchJobs` ≈ 0.4s+, `getCompany` variable. There is no global navigation
indicator anywhere in the app (`grep` for `$navigating` is empty), so the user
sees nothing move after clicking.

The company entity is needed for the header and SEO (`<title>`, canonical,
organization JSON-LD). The job list is a large, filterable, search-backed view
that is not needed for SEO.

## Goals / Non-Goals

**Goals:**
- Client navigation into a company page renders the header immediately; the job
  list fills in behind a skeleton.
- Any client-side navigation gives instant feedback via a global progress bar.
- Stop the load from fetching a full job it discards.
- Document where the API TTFB goes (Meili vs PG) so a later optimization is
  informed.

**Non-Goals:**
- Backend/API optimization, caching, or reindex — profiling only; any fix is a
  separate follow-up change.
- Changing the job-search view, filters, or the companies list page.
- Changing the API contract or response shapes.

## Decisions

### Stream the job list from `load` instead of awaiting it

`load` `await`s only `getCompany` and returns `searchJobs(...)` as an
**unresolved promise** (`initial`). SvelteKit streams promise values from a
server `load`: the resolved (`company`, `slug`) fields are sent first and the
navigation completes, then the `initial` promise streams in.

- **Why over a global spinner only:** a spinner tells the user "wait" but still
  shows a blank/old page. Streaming renders the real header immediately and only
  the job region waits — a faster *perceived* and *actual* transition.
- **Alternative — client-side `fetch` in `+page.svelte`:** moves the query to the
  browser and loses the SSR/SEO story for the shell. Rejected; streaming keeps
  SSR for the entity and is idiomatic SvelteKit.
- **SEO note:** the streamed job list is not in the first HTML flush on a direct
  SSR load. Acceptable: SEO-relevant content (company org JSON-LD, title,
  canonical) is built from the company entity, which stays synchronous. The job
  rows were never the SEO payload.

### Skeleton via `{#await}` in the view, reusing `skeleton.svelte`

`CompanyView` receives `initial` as a promise and wraps the job list in
`{#await data.initial}` → skeleton, `{:then slice}` → `JobsView`,
`{:catch}` → error state. Reuse the existing `web/src/lib/ui/skeleton.svelte`
primitive to compose a small job-list skeleton (a handful of row-shaped
placeholders), matching the existing `States.svelte`/async-load-state convention.

- The `{#key data.slug}` remount in `+page.svelte` stays: switching companies
  re-enters the pending→skeleton→rows cycle cleanly.

### Global navigation progress bar in the root layout

Add a thin top progress bar in `web/src/routes/+layout.svelte` shown while
`$navigating` is truthy. This is app-wide feedback for every route, not just
company pages, and is the cheapest fix for "I don't see anything happening."

- Keep it minimal (a CSS-animated bar), no external dependency.

### `getCompany` discarded-job cleanup — NOT REALIZABLE frontend-only

The load uses only `{ company }` from `getCompany`, discarding the returned job.
The intended cleanup was `getCompany(slug, 0, 0)` to fetch no jobs — but the API's
shared `pageParams` clamps `limit` to `[1, maxLimit]` (`max(limit, 1)`), so
`limit=0` becomes `limit=1` and one full job is still fetched and serialized.
`limit=1` is therefore already the minimal request; the load keeps it. Truly
avoiding the discarded job needs a backend change (a company-entity-only query,
or relaxing the clamp — which touches every list endpoint), which is out of this
change's frontend scope and is folded into the deferred latency follow-up (task
4.1). Chosen over changing the shared clamp here, to keep the change surgical.

### API latency profiling (documentation task)

Measure and record server-side TTFB for `GET /api/v1/companies/:slug` (PG:
`GetCompany` + `ListJobsByCompany`) and `GET /api/v1/jobs/search?company_slug=…`
(Meili). Capture where the time goes (network vs server, PG vs Meili) as notes in
the change; propose but do not implement an optimization.

## Risks / Trade-offs

- **Streamed content and crawlers** → SEO payload (JSON-LD/title/canonical) stays
  synchronous in the shell; only the non-SEO job rows stream. Low risk.
- **Skeleton/stream flicker on fast responses** → the skeleton may flash briefly
  when the search is fast. Acceptable; it is strictly better than a frozen page.
  No artificial delay added.
- **`{:catch}` must preserve the existing error UX** → route the streamed
  rejection to the same error state the app already uses, so a failed search
  still shows an error rather than a stuck skeleton.
- **No web test runner** → verify with `svelte-check` + `lint` and a manual/visual
  check; there are no unit tests to add for the Svelte components.
