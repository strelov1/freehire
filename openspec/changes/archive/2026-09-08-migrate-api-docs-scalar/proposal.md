## Why

`/docs/api` is a fully hand-rolled Svelte rendering of 253 endpoints (`web/src/lib/docs/api-spec.ts`, ~4400 lines): every endpoint's parameters, curl example, and response example is maintained by hand across dedicated per-endpoint routes. This is expensive to keep current and visually inconsistent, which is the stated dissatisfaction driving this change. A prior design (`openspec/changes/archive/2026-06-19-add-api-docs-page/design.md`) explicitly rejected an OpenAPI/Swagger-based renderer as a Non-Goal, reasoning that such tools "render interactive facet tables poorly." That reasoning no longer holds: Scalar (the current open-source OpenAPI reference renderer) builds a navigable sidebar and search index directly from Markdown headings in `info.description`, and renders GitHub-flavored Markdown tables, so the same cross-cutting content (error codes, pagination rule, filter vocabulary) that motivated the rejection now has a workable home.

## What Changes

- **BREAKING**: Replace the hand-rolled `/docs/api` Svelte pages with an embedded Scalar (`@scalar/api-reference`) reference, driven by a generated OpenAPI 3.1 document.
- Add a new generator script (sibling to the existing `gen:api-docs`) that transforms `web/src/lib/docs/api-spec.ts` into a new OpenAPI 3.1 static asset, distinct from and unrelated to `web/static/openapi.yaml` (the ChatGPT Actions schema, which stays untouched at its own 3.0.3 / ~9-path shape for GPT Actions importer compatibility).
- Extend `api-spec.ts`'s data model: a structured `deprecated?: {since, replacement}` field on `Endpoint` (does not exist today — deprecation is currently only implied in free-text prose), and structured Overview/Filtering section data (today free-form content only the landing Svelte page understands) that both generators (Markdown and OpenAPI) can consume.
- Retire `web/src/routes/docs/api/+page.svelte`, `+layout.svelte`, `DocsNav.svelte`, `DocsEndpoint.svelte`, and all `[group]/[endpoint]/+page.svelte` + `+page.server.ts` routes. The per-endpoint routes become 301 redirect stubs to `/docs/api`.
- Server-render the Scalar reference (via `renderApiReference()` + client hydration via `getJsAsset()`) so `/docs/api` keeps returning fully rendered HTML for SEO — no official SvelteKit recipe exists for this, so the wiring is custom.
- Theme the embedded reference from the design system's CSS custom properties (not Scalar's default preset theme), so it visually matches the rest of the site.
- `docs/API.md` keeps generating from `api-spec.ts` exactly as today; unaffected except for reading the new `deprecated` field where present.

## Capabilities

### New Capabilities

(none — this modifies how an existing capability's requirements are met, it does not introduce a new domain capability)

### Modified Capabilities

- `api-documentation`: the rendered `/docs/api` page changes from a fully hand-rolled Svelte UI to an embedded, OpenAPI-spec-driven Scalar reference; adds a generated OpenAPI 3.1 artifact as a second generator output alongside `docs/API.md`; adds structured `deprecated` and Overview/Filtering data to the typed source; the endpoint-detail page structure (per-endpoint SvelteKit routes) is removed in favor of Scalar's own in-page navigation.

## Impact

- `web/src/lib/docs/api-spec.ts` — extended data model (deprecated field, structured Overview/Filtering sections).
- `web/src/routes/docs/api/**` — landing, layout, nav, and per-endpoint routes rewritten or removed.
- New generator script (naming TBD in design) + new CI lint step (`@redocly/cli lint`) for the new OpenAPI artifact, separate from the existing lint of `web/static/openapi.yaml`.
- `web/package.json` — new dependency on `@scalar/api-reference`.
- Design system — new CSS custom property mapping consumed by the Scalar theme.
- Not affected: `web/static/openapi.yaml` (ChatGPT Actions schema), `docs/API.md` generator's existing behavior, ChatGPT Actions integration.
