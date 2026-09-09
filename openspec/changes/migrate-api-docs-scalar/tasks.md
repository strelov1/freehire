## 1. Data model

- [ ] 1.1 Add `deprecated?: { since: string; replacement: string }` to the `Endpoint` type in `web/src/lib/docs/api-spec.ts`
- [ ] 1.2 Mark the known deprecated endpoints (e.g. `/jobs/{slug}/fit` → `/jobs/{slug}/match-analysis`) with the new field
- [ ] 1.3 Restructure the Overview/Filtering content in `api-spec.ts` into structured heading+Markdown-body sections consumable by a generator (replacing whatever free-form shape the current landing page reads directly)
- [ ] 1.4 Run `pnpm gen:api-docs` (or equivalent) and confirm `docs/API.md` is unchanged in content, byte-identical aside from any new deprecated-note lines

## 2. OpenAPI generator

- [ ] 2.1 Add a new generator script (sibling to `scripts/gen-api-docs.mjs`) that transforms `api-spec.ts` into an OpenAPI 3.1 document
- [ ] 2.2 Map per-endpoint fields to `summary`/`description`/`parameters`/`requestBody`/`responses`/`tags`
- [ ] 2.3 Map the 5 auth levels to named `securitySchemes`, keeping "Moderator" and "Browser-extension-only" nuance in operation `description` prose
- [ ] 2.4 Map `deprecated` to native OpenAPI `deprecated: true` plus a description note naming the replacement
- [ ] 2.5 Assemble `info.description` from the structured Overview/Filtering sections (task 1.3), as Markdown with headings
- [ ] 2.6 Map endpoint groups to `tags` (and `x-tagGroups` for any coarser category)
- [ ] 2.7 Write the output to a new static asset with a name unambiguously distinct from `web/static/openapi.yaml` (e.g. `web/static/api-reference.openapi.json`), with a header comment stating its purpose and consumer
- [ ] 2.8 Add a golden-fixture test: a few representative `api-spec.ts` endpoints (simple GET no auth, query-param endpoint, body+auth endpoint) → expected OpenAPI fragment
- [ ] 2.9 Add a CI step running `@redocly/cli lint` against the new artifact, separate from the existing lint step for `web/static/openapi.yaml`

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
