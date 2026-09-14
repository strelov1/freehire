## Why

Wellfound (formerly AngelList Talent) is a large startup job marketplace with no adapter in
the catalogue today. Its listing/browse/search surface sits behind a Cloudflare JS challenge
that neither a plain HTTP client nor our own proxied headless-browser tier can pass (verified
live), but the hosted Firecrawl tier already wired for `bayt`/`gulftalent` does pass it, and
the pages it returns embed a full, structured Next.js/Apollo job payload — no further
per-posting fetch needed. A measured sample against the real `classify` dictionary shows a
healthy technical yield (18.8% confirmed technical, only 1.5% rejected), unlike the
near-total-non-technical shape that ruled out other Firecrawl-tier candidates. The source is
worth onboarding.

## What Changes

- Add a new `wellfound` source adapter to `internal/ingest/sources/` that reads Wellfound's
  own role-taxonomy search pages (e.g. `/role/r/software-engineer`) rather than the whole
  general-marketplace firehose, so the crawl stays scoped to genuinely technical slices.
- Wire `wellfound` into the existing `firecrawlProviders` map
  (`internal/ingest/sources/firecrawltier.go`) — the same hosted-tier pattern `bayt` and
  `gulftalent` already use — since no address this repository can egress from is served a
  real page.
- Parse each fetched page's embedded `__NEXT_DATA__` Next.js payload (a normalized Apollo
  GraphQL cache) for job and company data, rather than scanning rendered DOM — the listing
  already carries each posting's full HTML description inline, so the adapter is NOT a
  `HydratingSource` (no second, per-posting request).
- Paginate each role slice to the payload's own authoritative total rather than an
  empty-page heuristic.

## Capabilities

### New Capabilities
- `wellfound-source`: crawling Wellfound's role-scoped job search pages through the hosted
  Firecrawl tier and parsing the embedded Next.js/Apollo job payload into the catalogue.

### Modified Capabilities
(none — `source-ingest` and `board-harvest`'s existing requirements already cover "a new
provider is an adapter plus a registry line plus boards added via `cmd/add-board`"; adding
one more conforming provider does not change what either capability requires.)

## Impact

- **New file(s)**: `internal/ingest/sources/wellfound.go` (+ `wellfound_test.go`,
  `testdata/`).
- **Modified file(s)**: `internal/ingest/sources/registry.go` (register `wellfound`, present
  unconditionally like `bayt`/`gulftalent`/`jobleads`), `internal/ingest/sources/firecrawltier.go`
  (register `wellfound` in `firecrawlProviders`, which rewires that existing registry entry
  onto the hosted client when configured).
- **Dependencies**: requires `FIRECRAWL_API_KEY` to be configured (already is, in
  production) — without it the provider is still registered but every crawl attempt fails on
  the Cloudflare challenge response, the same shape `bayt`/`gulftalent` already have without
  their own credential.
- **Boards**: initial role-slice boards are added afterward via the normal
  `cmd/add-board`/curator flow; this change does not itself seed catalogue rows.
- **Out of scope**: resolving a listing's `atsSource` into a second copy via the actual ATS;
  a Wellfound-specific non-technical vocabulary (the generic `classify.ConfirmedNonTech` gate
  already measures healthy); cross-role-slice duplicate suppression beyond the existing
  duplicate-marker machinery; any Wellfound auto-apply capability.
