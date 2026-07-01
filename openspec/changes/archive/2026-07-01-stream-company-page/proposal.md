## Why

Clicking a company on `/companies` freezes the UI for ~1s+ with no visual
feedback: the `/companies/[slug]` SvelteKit `+page.server.ts` load `await`s
`Promise.all([getCompany, searchJobs])` before the client-side navigation renders
anything, and the app has no global navigation indicator. The user sees the old
page sitting still and cannot tell that anything is loading. The company entity
(header + SEO title) is cheap and ready quickly, but the whole transition is held
hostage by the slower Meilisearch job query.

## What Changes

- Split the company-page load so it `await`s only the fast `getCompany` (needed
  for the header and SEO `<title>`) and returns `searchJobs` as an **unresolved
  promise** that SvelteKit streams. The page shell renders as soon as the company
  entity is ready.
- Render a job-list **skeleton** while the streamed search results are pending,
  via `{#await data.initial}` in the company view, reusing the existing
  `web/src/lib/ui/skeleton.svelte`.
- Add a **global navigation progress indicator** in the root layout, driven by
  the `$navigating` store, so any client-side navigation (not just company pages)
  gives instant feedback the moment a link is clicked.
- Cleanup (investigated, deferred): the company-page load calls
  `getCompany(slug, 1, 0)` but uses only its `{ company }` — the returned job is
  discarded. A planned `limit=0` was found to be a no-op (the API clamps `limit`
  to `>= 1`), so truly dropping the discarded job needs a backend change; it is
  folded into the deferred latency follow-up rather than done here.
- Investigate and document the server-side TTFB of `searchJobs` (Meilisearch,
  ~0.4s+) and `getCompany` (Postgres) to pin the latency source; actual backend
  optimization is tracked but deferred out of this change.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `web-frontend`: the company detail page streams its job list behind a skeleton
  instead of blocking the whole navigation on the search query, and the app gains
  a global navigation progress indicator so client-side navigations give instant
  feedback.

## Impact

- **Frontend**: `web/src/routes/companies/[slug]/+page.server.ts` (split
  load, stream `initial`, `getCompany` `limit=0`); `web/src/lib/components/CompanyView.svelte`
  (job list wrapped in `{#await}` with a skeleton); `web/src/routes/+layout.svelte`
  (add `$navigating`-driven progress bar); possibly a small
  `web/src/lib/components/` skeleton/progress component.
- **Backend**: no code change. A profiling task documents `searchJobs` /
  `getCompany` TTFB; any resulting optimization is a separate follow-up.
- **Verification**: `web/` has no test runner — verify via `svelte-check` +
  visual check; keep changes surgical.
- **No schema, no reindex, no API contract change.**
