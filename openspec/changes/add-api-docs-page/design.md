## Context

freehire is a public IT-job aggregator with a Fiber HTTP API (`/api/v1`) and a
SvelteKit SSR frontend (`web/`). The API's read surface is rich and filterable —
`/jobs/search` accepts ~18 string facets (the `internal/search.StringFacets`
map), plus `_mode=and`/`_exclude` modifiers, numeric ranges, a boolean visa
filter, full-text `q`, sort, and a semantic ratio — but none of it is
documented for external consumers. The only references today are the source and
incidental hints on the `/cli` page.

The filter vocabulary already has a single Go source of truth (`StringFacets`)
that is code-generated into TS (`web/src/lib/generated/contracts.ts` via
`make gen-contracts`) and humanized in `web/src/lib/facets.ts`. Content pages
(`/cli`, `/recruiters`) follow a simple pattern: a route that renders a View
component; copy-to-clipboard already exists in `CliView`/`ApiKeysView`.

The user wants both a website page and a repo `docs/API.md`, covering the whole
public API, as a static reference with curl examples.

## Goals / Non-Goals

**Goals:**
- One typed data source (`api-spec.ts`) → rendered page **and** generated
  `docs/API.md`; no drift between the two.
- The job-search filter table derives from existing generated contracts +
  `facets.ts`, so it never drifts from the Go `StringFacets` either.
- Cover the whole public API, with depth on filtering jobs.
- SSR page for SEO, consistent with existing content-page styling.

**Non-Goals:**
- No OpenAPI/Swagger spec, no interactive "try-it" console, no markdown renderer
  in the SPA.
- No backend/API behavior changes.
- No CI drift-check for `docs/API.md` (noted as a future seam; the generator
  exists, enforcing it in CI is a separate decision).
- No new runtime dependencies.

## Decisions

### Docs-as-data: one typed source, two renderers
Describe the API in `web/src/lib/docs/api-spec.ts` as typed structures
(`Group → Endpoint → Param`, with `auth`, `curl`, and `responseExample` as
strings so they drop verbatim into Markdown). The Svelte page renders from it;
`web/scripts/gen-api-docs.mjs` imports it and writes `docs/API.md`.

*Alternatives considered:* two independent hand-written copies (rejected —
guaranteed drift); markdown-first with an in-SPA renderer (rejected — pulls a
markdown dependency into the SPA and renders interactive facet tables poorly).

### Filter vocabulary sourced from generated contracts
`web/src/lib/docs/filters.ts` builds the facet table from
`web/src/lib/generated/contracts.ts` value arrays and `facets.ts` labels, plus a
small hand-written list of the non-facet modifiers (`_mode`, `_exclude`, numeric
ranges, `visa_sponsorship`, `q`, `sort`/`order`, `semantic_ratio`) that have no
generated counterpart. This keeps the closed-vocabulary part lock-stepped with
Go while documenting the modifiers that only live in `query_filter.go`.

### Generator runs on the existing Vite toolchain
`api-spec.ts` is TypeScript, so the generator imports it via `vite-node` (Vite is
already a dependency) rather than adding `tsx`. Wire it as `"gen:api-docs"` in
`web/package.json`, mirroring `make gen-contracts`. The generated `docs/API.md`
carries a "generated — do not edit; run `npm run gen:api-docs`" header.

*Alternative considered:* a Go generator like `cmd/gen-contracts` (rejected — the
spec content is TS-native and has no Go origin; keeping it in the web toolchain
avoids a Go↔TS round-trip).

### Page structure
Route `web/src/routes/docs/api/+page.svelte` + `+page.ts` (title/meta) rendering
a new `DocsApiView.svelte`: sticky table-of-contents, anchored sections per
group, parameter tables, and code blocks with the existing copy button. Styling
matches the other content pages (Tailwind). Nav link "API" added to
`TopBar.svelte`; cross-links from `CliView`/`ApiKeysView`.

## Risks / Trade-offs

- **Page and generated MD could still drift if someone hand-edits `API.md`** →
  Mitigation: the generated header warns against it; a CI check is a documented
  future seam.
- **`vite-node` invocation differences across environments** → Mitigation: keep
  the generator a plain ESM script invoked through the project's local Vite; the
  generated `docs/API.md` is committed, so consumers never need to run it.
- **Spec data going stale as the API evolves** → Mitigation: the closed-
  vocabulary filter table is derived (auto-updates); only prose/endpoint shape is
  hand-maintained, the same maintenance burden as any reference doc.
