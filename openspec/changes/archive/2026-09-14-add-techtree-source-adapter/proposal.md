## Why

TechTree (`jobs.techtree.dev`) is a small AI-recruiting-agency job board with no adapter in
the catalogue today — confirmed by a user-supplied posting URL that matched nothing in
`internal/ingest/sources/` or `internal/ingest/atsboard/`. It is multi-tenant (one posting
sampled was for "Telepatia", a healthcare AI startup) and its own `sitemap.xml` lists
exactly 100 open postings as of 2026-09-14 — a modest catalogue, not a major board, so this
is a small-value adapter and should be scoped and prioritized as one, not oversold.

**Correction from this proposal's first draft**: an initial static-only inspection (plain
`curl`, truncated to the first 3000 bytes, plus a grep pass that turned out unreliable on
this page's single very long line) concluded the detail page was an unrendered
client-side-only SPA needing the headless-browser tier. A follow-up spike — a full read of a
fresh fetch, and independently, the repository's own `internal/platform/browser` in-page
fetch — shows the opposite: the page is fully **server-rendered** (a TanStack Start-style
SSR build, judging by its `$tsr` hydration-stream script and file-based route chunk names)
and already carries a `<script type="application/ld+json">` `schema.org/JobPosting` block
plus the full rich-text description in the DOM. No browser tier, proxy, or hosted-fetch tier
is needed — a plain HTTP client is enough, the same shape `internal/ingest/sources/thehub.go`
already uses for a near-identical site (flat sitemap + per-page ld+json JobPosting).

## What Changes

- Add a new `techtree` source adapter to `internal/ingest/sources/` that enumerates postings
  from `https://jobs.techtree.dev/sitemap.xml` (a flat `<urlset>`, not an index — the only
  listing surface found; there is no board-scoped or paginated listing endpoint) and reads
  each posting's own detail page over the plain shared HTTP client.
- Each detail page server-renders a `schema.org/JobPosting` ld+json block (title,
  `hiringOrganization.name`, `jobLocation.address` — emitted as a plain string, not a
  structured `PostalAddress`, so it needs its own decode type rather than the shared
  `schemaAddress` — `datePosted`, `employmentType`). The ld+json block's own `description`
  field is short (the same text as the page's meta description), **not** the full posting
  body, so the full rich-text description is read separately from the DOM's own `prose`-class
  container, the same "structured fields from ld+json, full body from a DOM class match"
  split `broadpeak`'s adapter already uses when its embedded state carries no description.
- Register `techtree` as an aggregator-marker, boardless source (each posting names its own
  hiring company, read from the page, not from a per-tenant `CompanyEntry.Company` — the same
  shape `thehub`/`gulftalent`/`wellfound` already use), since the job URL carries no tenant
  slug at all (`/job/<job-uuid>?tp=<tracking-uuid>` — the `tp` param appears on every page
  including the "All jobs" link, so it reads as a referral/tracking id, not a tenant
  identifier; the closest existing precedent for "no board encoded in the job URL" is the
  Gusto entry in `internal/ingest/atsboard/board.go`).

## Capabilities

### New Capabilities
- `techtree-source`: crawling TechTree's sitemap-enumerated job postings over the plain HTTP
  client and parsing each posting's server-rendered ld+json JobPosting plus its DOM
  description into the catalogue.

### Modified Capabilities
(none — `source-ingest` and `board-harvest`'s existing requirements already cover "a new
provider is an adapter plus a registry line plus boards added via `cmd/add-board`"; adding
one more conforming provider does not change what either capability requires.)

## Impact

- **New file(s)**: `internal/ingest/sources/techtree.go` (+ `techtree_test.go`,
  `testdata/`).
- **Modified file(s)**: `internal/ingest/sources/registry.go` (add `NewTechTree(c)` to
  `sources.All`, alongside the other boardless multi-company aggregators such as `thehub`,
  `compleo`, `instaffo`).
- **Dependencies**: none — the plain shared `HTTPClient` (`XMLGetter` + `HTMLGetter`)
  already used by every unmetered adapter is sufficient. No proxy, browser, or hosted-fetch
  tier.
- **Boards**: boardless (no board segment in the job URL, no per-tenant catalog rows this
  change would seed) — the whole site is one crawl, same as `thehub`.
- **Out of scope**: resolving whether a TechTree posting duplicates a first-party ATS
  posting already in the catalogue (left to the existing cross-source dedup machinery, same
  posture `wellfound`'s `atsSource` deferral takes); any TechTree-specific non-technical
  title vocabulary (use the generic `classify.ConfirmedNonTech` gate until measurement shows
  a gap); any auto-apply capability for TechTree's own apply flow.
