## 1. Data source

- [x] 1.1 Create `web/src/lib/docs/api-spec.ts`: the typed model (`Param`,
  `Endpoint` with `method`/`path`/`auth`/`query`/`body`/`pathParams`/`curl`/
  `responseExample`, `Group`) and the content for every public endpoint group
  (intro, jobs, companies, auth, api-keys, per-user interactions, submissions,
  reports, saved-searches/subscriptions).
- [x] 1.2 Create `web/src/lib/docs/filters.ts`: build the job-search filter table
  from `generated/contracts.ts` value arrays + `facets.ts` labels, plus the
  hand-listed non-facet modifiers (`_mode=and`, `_exclude`, `salary_min/max`,
  `experience_years_min`, `visa_sponsorship`, `q`, `sort`/`order`,
  `semantic_ratio`). Export worked filter recipes.

## 2. Markdown generator

- [x] 2.1 Create `web/scripts/gen-api-docs.mjs`: import the spec data via
  vite-node, render `docs/API.md` (generated-header + all groups/endpoints/filter
  table), write to repo root `docs/API.md`.
- [x] 2.2 Add `"gen:api-docs"` script to `web/package.json` and a
  `"gen:api-docs:smoke"` (or reuse `og:smoke` style) that runs the generator
  twice and asserts the two outputs are byte-identical (idempotency).
- [x] 2.3 Run the smoke check; confirm `docs/API.md` is produced with the
  generated header and is stable across runs.

## 3. Page and component

- [x] 3.1 Create `web/src/lib/components/DocsApiView.svelte`: sticky table of
  contents, anchored sections per group, parameter tables, the job-search filter
  table, and copy-enabled code blocks (reuse the `CliView`/`ApiKeysView` copy
  pattern). Tailwind styling consistent with other content pages.
- [x] 3.2 Create `web/src/routes/docs/api/+page.svelte` (renders `DocsApiView`)
  and `+page.ts` (title + meta description; ensure SSR, no auth).

## 4. Navigation and cross-links

- [x] 4.1 Add an "API" link to `web/src/lib/components/TopBar.svelte` (desktop +
  mobile nav) pointing to `/docs/api`.
- [x] 4.2 Cross-link the docs page from `CliView.svelte` and `ApiKeysView.svelte`
  as the full API reference.

## 5. Generate and verify

- [x] 5.1 Run `npm run gen:api-docs`; commit the generated `docs/API.md`.
- [x] 5.2 Run `npm run check` (svelte-check) and `npm run lint` on the new files;
  confirm no new errors versus the baseline.
- [x] 5.3 Visually verify `/docs/api` renders server-side in dev and that the
  page's coverage matches the generated `docs/API.md` (manual cross-check).
