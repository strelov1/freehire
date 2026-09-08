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

- [ ] 3.1 Add `@scalar/api-reference` to `web/package.json`
- [ ] 3.2 Build a `+page.svelte` for `/docs/api` that mounts the Scalar web component (`@scalar/api-reference/component.js`) inside the existing site layout, pointed at the generated OpenAPI artifact
- [ ] 3.3 Wire server-side rendering via `renderApiReference()` and client hydration via `getJsAsset()`, following Scalar's generic Node-server integration pattern (no official SvelteKit recipe exists)
- [ ] 3.4 Verify via view-source that the server response contains real endpoint content, not an empty mount point
- [ ] 3.5 Map design-system CSS tokens (`border-border`, `bg-secondary`, `text-muted-foreground`, `text-brand-strong`, `bg-brand-muted`, etc.) onto Scalar's theme CSS custom properties, replacing any default preset theme

## 4. Retire the old implementation

- [ ] 4.1 Replace `web/src/routes/docs/api/[group]/[endpoint]/+page.svelte` and its `+page.server.ts` with a redirect stub returning HTTP 301 to `/docs/api`
- [ ] 4.2 Delete `web/src/routes/docs/api/+layout.svelte`, `DocsNav.svelte`, and `DocsEndpoint.svelte`
- [ ] 4.3 Delete the old landing `+page.svelte`/`+page.server.ts` content (Shiki-highlighting setup included) once step 3 replaces it
- [ ] 4.4 Run `pnpm check:dead` (knip) and `deadcode -test -tags=integration,llmlive ./...` where applicable, and confirm no orphaned files remain

## 5. Verification

- [ ] 5.1 Manually verify in a browser: light theme, dark theme
- [ ] 5.2 Manually verify several endpoints of varying complexity: no-auth GET, query-param GET, body+cookie-auth POST, an SSE endpoint
- [ ] 5.3 Manually verify mobile viewport width
- [ ] 5.4 Verify the 301 redirects work for a sample of legacy per-endpoint URLs
- [ ] 5.5 Verify the Overview/Filtering sections (pagination rule, error table, "What is not here", filter modifiers table, recipes) appear in the sidebar/search and render correctly as Markdown
- [ ] 5.6 Confirm `web/static/openapi.yaml` and the ChatGPT Actions integration are untouched (diff check)
