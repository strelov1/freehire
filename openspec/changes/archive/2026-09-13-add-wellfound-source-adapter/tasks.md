## 1. Fixtures

- [x] 1.1 Following this codebase's own convention for a self-contained embedded payload
      (see `remotedotcom_test.go`'s `remotedotcomListFlight` helper) rather than a large
      captured-HTML file: add a small `wellfound_test.go` helper that wraps a given
      `apolloState`-shaped JSON literal in the real minimal HTML shape
      (`<script id="__NEXT_DATA__" type="application/json">...</script>`), grounded in the
      exact field names and nesting verified live against `/role/r/software-engineer` during
      this change's spike.
- [x] 1.2 Write the literal `apolloState` JSON fixtures the tests need, covering: a normal
      `JobListingSearchResult` entry referenced by some `StartupResult.highlightedJobListings`,
      one with no compensation stated, one NOT referenced by any `StartupResult` at all — the
      real "unresolvable company" shape, since the reference is reverse-only — one whose entry
      is malformed/incomplete, and the `ROOT_QUERY.talent` → `seoLandingPageJobSearchResults(...)`
      → `pageCount`/`totalJobCount` field this change's spike confirmed live.
- [x] 1.3 Add a fixture page whose `<script id="__NEXT_DATA__">` body is present but not valid
      JSON (or missing the expected `apolloState` shape), for the "unparseable page fails
      loudly" scenario.

## 2. `__NEXT_DATA__` / Apollo cache parsing

- [x] 2.1 Write a failing test asserting that, given the role-search fixture's HTML, the
      parser extracts the `__NEXT_DATA__` script body and decodes
      `props.pageProps.apolloState.data`.
- [x] 2.2 Implement the extraction + decode, returning a typed error (not a panic or a
      silent empty map) when the script tag is absent or its JSON does not decode.
- [x] 2.3 Write a failing test asserting the parser walks the normalized cache and returns
      one entry per `JobListingSearchResult:*` key on the role-search fixture, with its
      scalar fields (`title`, `description`, `compensation`, `remote`/`remoteConfig`,
      `locationNames`, `id`, `slug`) populated into an adapter-internal struct (not yet
      `sources.Job` — that mapping is step 4).
- [x] 2.4 Implement that walk.
- [x] 2.5 Write a failing test asserting a reverse index built from every `StartupResult:*`
      entry's `highlightedJobListings[].__ref` correctly maps a job id to its startup's
      `name`, and that a job id referenced by no `StartupResult` resolves to no company.
- [x] 2.6 Implement that reverse-index build and lookup.
- [x] 2.7 Write a failing test for the "unparseable page" fixture (1.3) asserting the parser
      returns an error rather than zero postings.

## 3. Board URL and pagination

- [x] 3.1 Write a failing test asserting the adapter builds a role slice's page-N URL from
      `CompanyEntry.Board` (the role slug) and a page number.
- [x] 3.2 Implement the URL builder.
- [x] 3.3 Write a failing test asserting pagination stops at the payload's own stated
      total/page count rather than only on an empty page (use the role-search fixture's
      real total, or a small crafted one if the real figure would need an unreasonably
      large fixture).
- [x] 3.4 Implement the pagination loop against the (test-injected) fetch function.

## 4. `sources.Job` mapping and identity

- [x] 4.1 Write a failing test asserting a well-formed listing entry maps to a `sources.Job`
      with `ExternalID` set from Wellfound's own numeric id, `URL` built from that id and
      slug, `Description` as the entry's own HTML (sanitized through the shared
      `sanitizeHTML` helper other adapters use), and `Company` from the entry's hiring
      startup.
- [x] 4.2 Implement that mapping.
- [x] 4.3 Write a failing test asserting an entry with no resolvable hiring-company name is
      dropped rather than mapped with an empty `Company`.
- [x] 4.4 Implement that drop, and confirm (existing fixture from 1.2) it does not affect
      the other entries on the same page.
- [x] 4.5 Write a failing test asserting an entry with no extractable numeric id is dropped
      the same way.
- [x] 4.6 Implement that drop.

## 5. Adapter wiring

- [x] 5.1 Implement `Provider() string` returning `"wellfound"`.
- [x] 5.2 Implement `Fetch(ctx, e CompanyEntry) ([]Job, error)` composing the URL builder
      (3.2), pagination (3.4), per-page parsing (2.x), and per-entry mapping (4.x); a page
      that fails to fetch or parse returns an error for the whole `Fetch` call, matching the
      spec's "unparseable page is a loud failure" requirement.
- [x] 5.3 Register the adapter as `aggregator` (marker interface, see `registry.go`) — no
      `boardless`/`HydratingSource`/`fullBoardListing` markers, per design.md's decisions.
- [x] 5.4 Add `wellfound_test.go` covering `Fetch` end-to-end against the role-search
      fixture (HTTP layer stubbed), asserting the full set of scenarios from
      `specs/wellfound-source/spec.md`.

## 6. Registry and hosted-tier integration

- [x] 6.1 Register `"wellfound"` unconditionally in `internal/ingest/sources/registry.go`'s
      `All` (over the plain shared client, the same generic `HTMLGetter` most keyless
      adapters use — not the `fingerprintHTTP` special transport, which addresses a
      different problem, a rejected TLS/HTTP2 fingerprint, than Wellfound's full JS
      challenge), on BOTH the `c == nil` taxonomy path and the real-client path — mirroring
      why `jobleads` registers on both (`AggregatorProviders`/cross-source dedup must know
      about it even when nothing can currently crawl it).
- [x] 6.2 Add a `"wellfound"` entry to `firecrawlProviders` in
      `internal/ingest/sources/firecrawltier.go`, following the existing `bayt`/`gulftalent`
      shape — note `ApplyFirecrawlEgress` only REWIRES an existing registry key, so 6.1 must
      land first or this has nothing to rewire.
- [x] 6.3 Confirm (by reading, not by live-testing in CI) that an unconfigured
      `FIRECRAWL_API_KEY` leaves `wellfound` registered but unable to crawl anything (every
      request 403s on the Cloudflare challenge response) — the same shape `bayt`/`gulftalent`
      already have without their own credential, never a silent absence.

## 7. Verification

- [x] 7.1 `gofmt -l .` clean on every touched file.
- [x] 7.2 `go vet ./...` and `go test ./...` green.
- [x] 7.3 Re-read `specs/wellfound-source/spec.md` scenario by scenario and confirm each has
      a corresponding test from sections 2-5.
- [x] 7.4 Run the `simplify` pass over the diff.
