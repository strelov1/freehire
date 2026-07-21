## Why

The just-shipped "openings across locations" section renders the whole cluster as one
flat list (up to the page cap) below the job — for a 928-city role this is an
overwhelming wall of rows, and it sits separately from the existing "Similar jobs"
block. It should be compact and consistent with Similar jobs.

## What Changes

- "Similar jobs" and "Other locations" become **two tabs** in one related-content
  section under the job (a new `JobRelated` component), replacing the standalone
  `JobCopies` section and the inline Similar list.
- The Copies tab shows only the **first 10** locations plus a "View all N locations →"
  link; the full paginated list lives on a **dedicated page** `/jobs/:slug/copies`.
- The detail loader fetches only the small preview (10) for the tab; the full page
  loads pages of copies via the existing `limit`/`offset` + `meta.total`.

## Capabilities

### Modified Capabilities
- `job-cluster-copies`: the copies are presented as a tab with a bounded preview and a
  dedicated full-list page, instead of a full inline list.

## Impact

- `web/src/lib/components/JobRelated.svelte` (new), `JobCopies.svelte` (removed).
- `web/src/routes/jobs/[slug]/+page.svelte` + `+page.server.ts` (preview fetch).
- `web/src/routes/jobs/[slug]/copies/` (new full-list page + loader).
- `web/src/lib/api.ts` — `getJobCopies` gains `limit`/`offset`.
- No backend change (the endpoint already paginates and returns `meta.total`).
