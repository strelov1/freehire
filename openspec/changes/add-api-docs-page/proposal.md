## Why

freehire exposes a rich public read API — filterable job search, facet counts,
companies, plus auth/API-key and per-user endpoints — but the only documentation
is the source code and scattered hints on the CLI page. Developers (and AI
agents) who want to query jobs by filters have no single reference for the base
URL, the response envelope, the ~18 search facets, or the filter modifiers
(`_mode=and`, `_exclude`, numeric ranges). This change gives them one.

## What Changes

- Add a public, server-rendered documentation page at `/docs/api` covering the
  whole public API, with a heavy focus on querying jobs by filters (the
  `/jobs/search` + `/jobs/facets` vocabulary and recipes).
- Introduce the documentation as **data, not prose**: a single typed source
  (`web/src/lib/docs/api-spec.ts`) drives both the rendered page and a generated
  `docs/API.md`, so the two cannot drift.
- Derive the filter-vocabulary table from the existing generated contracts
  (`web/src/lib/generated/contracts.ts`) and `web/src/lib/facets.ts`, so the
  documented facets stay in lock-step with the Go `StringFacets` source of truth.
- Add a `gen:api-docs` npm script that regenerates `docs/API.md` from the spec
  data (mirroring the existing `make gen-contracts` pattern).
- Link the page from the top navigation and cross-link it from the CLI and
  API-keys pages.

No backend behavior changes — this is documentation of the existing API only.

## Capabilities

### New Capabilities
- `api-documentation`: a public, server-rendered reference for the freehire HTTP
  API generated from a single typed data source, rendered both as a website page
  and a repo `docs/API.md`, covering endpoints, the response envelope, and the
  job-search filter vocabulary.

### Modified Capabilities
<!-- None: the API surface itself is unchanged; this change only documents it. -->

## Impact

- **New frontend route**: `web/src/routes/docs/api/` (+page.svelte, +page.ts) and
  a `DocsApiView` component.
- **New data/source modules**: `web/src/lib/docs/api-spec.ts`,
  `web/src/lib/docs/filters.ts`.
- **New generator**: `web/scripts/gen-api-docs.mjs` + `gen:api-docs` script in
  `web/package.json`; new committed output `docs/API.md`.
- **Navigation**: `web/src/lib/components/TopBar.svelte`; cross-links in
  `CliView.svelte` and `ApiKeysView.svelte`.
- No Go, DB, or API changes; no new runtime dependencies (generator runs via the
  existing Vite toolchain).
