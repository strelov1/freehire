## 1. Global navigation indicator

- [x] 1.1 Add a top progress bar to `web/src/routes/+layout.svelte` shown while the
  `$navigating` store is truthy; CSS-animated, no external dependency; verify with
  `svelte-check` and a visual check that it appears on a client-side navigation and
  clears when it settles.

## 2. Stream the company job list

- [x] 2.1 Split `web/src/routes/companies/[slug]/+page.server.ts` load: `await`
  only `getCompany` and return `searchJobs(...)` as an unresolved promise
  `initial`; keep `company`, `initial`, `slug` in the returned data. NOTE: the
  planned `limit=0` cleanup is a no-op — the API's `pageParams` clamps `limit` to
  `>= 1`, so `getCompany` still fetches one discarded job; `limit=1` is already
  minimal. A true entity-only fetch needs a backend change, folded into task 4.1.
- [x] 2.2 Update `web/src/lib/components/CompanyView.svelte` to accept `initial` as
  a promise and render the job list via `{#await data.initial}` → job-list skeleton,
  `{:then slice}` → `JobsView`, `{:catch}` → the existing error state; keep the
  `{#key data.slug}` remount behavior.
- [x] 2.3 Compose a job-list skeleton (row-shaped placeholders) reusing
  `web/src/lib/ui/skeleton.svelte`, matching the existing async-load-state
  convention; verify with `svelte-check`.

## 3. Verify

- [x] 3.1 Run `svelte-check` and lint over `web/`; confirm no new errors from the
  changed files; visually verify the header appears immediately and the job list
  streams in behind the skeleton on both a direct load and a client navigation.

## 4. Profile API latency (documentation)

- [x] 4.1 Measure and document server-side TTFB for `GET /api/v1/companies/:slug`
  (PG: `GetCompany` + `ListJobsByCompany`) and
  `GET /api/v1/jobs/search?company_slug=…` (Meili), separating network from server
  and PG from Meili; record findings and a proposed (deferred) optimization in the
  change notes.
