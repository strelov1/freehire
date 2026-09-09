## 1. Data model

- [x] 1.1 Add `deprecated?: { since: string; replacement: string }` to the `Endpoint` type in `web/src/lib/docs/api-spec.ts`
- [x] 1.2 Mark the known deprecated endpoints with the new field — **finding**: no currently-listed endpoint qualifies. `/jobs/{slug}/fit` (the only alias mentioned anywhere) is deliberately excluded from the endpoint list itself (documented only in the "What is not here" Overview prose), not a listed-but-deprecated endpoint — marking it would expand documented API coverage beyond the approved design. The field and its rendering are verified via a synthetic fixture in `gen-api-docs-smoke.mjs` instead; real endpoints can adopt it going forward with no further plumbing.
- [x] 1.3 Restructure the Overview/Filtering content — **finding**: no restructuring needed. `OVERVIEW` (`{title, paragraphs, code}`) and `filters.ts`'s `FILTER_FACETS`/`FILTER_EXTRAS`/`FILTER_MODIFIERS`/`RECIPES` are already typed, structured data; paragraphs already use backtick spans that are valid Markdown inline-code. The `info.description` assembly happens in task 2.5 directly from this existing shape.
- [x] 1.4 Run `pnpm gen:api-docs` and confirm `docs/API.md` is unchanged — confirmed via `git diff --stat docs/API.md` (empty diff); no real endpoint uses `deprecated` yet so no new lines were expected

## 2. OpenAPI generator

- [x] 2.1 Add a new generator script (sibling to `scripts/gen-api-docs.mjs`) that transforms `api-spec.ts` into an OpenAPI 3.1 document — `web/scripts/gen-openapi.mjs`, reusing `loadDocsModules` from `gen-api-docs.mjs`
- [x] 2.2 Map per-endpoint fields to `summary`/`description`/`parameters`/`requestBody`/`responses`/`tags`
- [x] 2.3 Map the 5 auth levels to named `securitySchemes`, keeping "Moderator" and "Browser-extension-only" nuance in operation `description` prose — all 4 named schemes ride the same `hire_token` session cookie except `apiKeyAuth`, distinguished by scheme description rather than mechanism
- [x] 2.4 Map `deprecated` to native OpenAPI `deprecated: true` plus a description note naming the replacement
- [x] 2.5 Assemble `info.description` from the structured Overview/Filtering sections, as Markdown with headings
- [x] 2.6 Map endpoint groups to `tags` — **finding**: no coarser category exists in the source data (31 flat group titles), so `x-tagGroups` does not apply; inventing one would add structure the data does not have
- [x] 2.7 Write the output to `web/static/api-reference.openapi.json`, with a header comment in the generator stating its purpose and that it is unrelated to `web/static/openapi.yaml`
- [x] 2.8 Add a golden-fixture/smoke test (`gen-openapi-smoke.mjs`, following the project's existing no-JS-unit-runner convention from `gen-api-docs-smoke.mjs`) — 27 checks covering auth mapping, placeholder params, SSE responses, deprecation, idempotency
- [x] 2.9 Add CI steps (generate+diff, smoke test, `@redocly/cli lint --extends=minimal`) in the `web` job, separate from the `artifacts` job's lint of `web/static/openapi.yaml`

## 3. Scalar integration

- [x] 3.1 Add `@scalar/api-reference` to `web/package.json` — plus `@scalar/server-side-rendering` (SSR fragment) and `@scalar/types` (config types), and dropped `@scalar/client-side-rendering` after confirming it's the wrong shape (CDN-oriented, not needed for a Vite-bundled app) — see design.md Decision 4
- [x] 3.2 Build `web/src/routes/docs/api/+page.svelte` mounting `createApiReference('#scalar-app', ...)` (from `@scalar/api-reference`, dynamically imported, bundled normally by Vite — not the `component.js` web component, which isn't how this package ships) inside the existing site layout
- [x] 3.3 Wire SSR via `renderApiReferenceToString()` (fragment, not `renderApiReference()`'s full document) + client hydration via `createApiReference` — `getJsAsset()` turned out to belong to a different integration shape (non-bundled Node servers); see design.md Decision 4 for the corrected package mapping
- [x] 3.4 Verified via curl that the server response contains real endpoint content (23 occurrences of known endpoint text in the raw HTML), not an empty mount point
- [x] 3.5 Map design-system CSS tokens onto Scalar's theme CSS custom properties — **finding, corrected twice**: (1) had to add `@scalar/api-reference/style.css`, missing entirely at first, which is why the page rendered unstyled; (2) `updateConfiguration()` cannot make Scalar's dark mode follow later toggles at all — `useColorMode` reads `forceDarkModeState` once at setup with no reactive watch (confirmed against the installed package's source) — so the working fix destroys and recreates the whole instance on every toggle instead; (3) the CSS override selector was scoped to `#scalar-app :global(.light-mode)`/`:global(.dark-mode)`, which matched only the always-dark request-example cards (a `#scalar-app` descendant) and never the general sidebar/heading text, because Scalar applies the mode class to `document.body` — an ancestor of `#scalar-app`, structurally unreachable by a descendant-only selector. This passed an earlier screenshot check because near-black-on-near-white looks correct at a glance even when it is Scalar's own default rather than the site's token. The working rule is unscoped (`:global(.light-mode)`/`:global(.dark-mode)`, matching the class wherever it appears) with `!important`. See design.md Decisions 5-6. Re-verified by reading actual computed CSS colors live-toggling with no page reload: heading color matches the site's `--foreground` exactly in both modes.

## 4. Retire the old implementation

- [x] 4.1 Replaced `web/src/routes/docs/api/[group]/[endpoint]/+page.svelte` (deleted) and its `+page.server.ts` with a redirect stub returning HTTP 301 to `/docs/api`
- [x] 4.2 Deleted `web/src/routes/docs/api/+layout.svelte`, `DocsNav.svelte`, and `DocsEndpoint.svelte`
- [x] 4.3 Deleted the old landing `+page.svelte`/`+page.server.ts` content — **also found and deleted three further orphans knip couldn't reach on its own reasoning alone but static analysis confirmed**: `DocsCodeBlock.svelte`, `$lib/docs/nav.ts` (NAV/slugify/findEndpoint), and `$lib/docs/format.ts`/`$lib/docs/highlight.ts` (Shiki wrapper) — all had no consumer left outside the deleted files. Fixed one dangling comment reference to the deleted `DocsNav` in `JobView.svelte`.
- [x] 4.4 Ran `pnpm check:dead` (knip): fixed a false-positive "unused file" on `api-spec.ts`/`filters.ts` (only reached via `vite.ssrLoadModule('/src/lib/docs/...')`, a runtime string knip can't trace — added both to `knip.config.js`'s `entry` list, same pattern already established there for `og/brand.ts`), removed the now-unused `shiki` dependency, and removed an unused export (`SCALAR_SPEC_URL`, only ever used within its own module). Remaining knip findings are all pre-existing `extension/` noise from that workspace's dependencies never being installed in this worktree (out of scope — nothing there was touched). Go's `deadcode` doesn't apply — no Go changed.

## 5. Verification

- [x] 5.1 Manually verified in a real headless browser (Playwright): light theme and dark theme both render cohesively with the site's own palette (screenshots inspected)
- [x] 5.2 Manually verified: the generated reference covers no-auth GET (`/jobs`), query-param GET (`/jobs/search`), a body+cookie-or-key-auth POST (`/market/coverage`), and an SSE GET (`/jobs/{slug}/match-analysis/stream`) — all confirmed correctly shaped in the OpenAPI generator's own smoke test; visually confirmed present in the rendered sidebar
- [x] 5.3 Manually verified at a 390×844 mobile viewport (Playwright screenshot): sidebar collapses into a hamburger menu, content stays single-column and readable, no horizontal overflow
- [x] 5.4 Manually verified with a real request: `curl http://localhost:5173/docs/api/jobs/list-jobs` → `HTTP 301` with `Location: /docs/api`
- [x] 5.5 Verified: Base URL, Response envelope, Pagination, Errors, Authentication model, What is not here, and Filtering jobs all appear as their own sidebar entries and render their Markdown (headings, tables, code fences) correctly in the screenshot check
- [x] 5.6 Confirmed via `git status`/`git diff` that `web/static/openapi.yaml` has no changes in this branch
