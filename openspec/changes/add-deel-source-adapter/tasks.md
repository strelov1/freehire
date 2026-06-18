## 1. Test fixture — trimmed real board page

- [x] 1.1 Build `internal/sources/testdata/deel_klarna.html` from the real `jobs.deel.com/klarna`
  page, trimmed to ~2 postings plus their `$N` text rows and the `careerPageSettings`, keeping
  the flight format intact (faithful `$N`→`T<byte-length>` resolution against real-shaped data)
- [x] 1.2 Add a `deel_empty.html` fixture: a board page whose `jobPostings` payload is empty

## 2. Flight payload extraction (TDD)

- [x] 2.1 Test a helper that concatenates and JS-string-decodes the page's `self.__next_f.push`
  chunks into one flight stream, asserting a known multibyte description substring decodes as
  correct UTF-8 (no mojibake)
- [x] 2.2 Test ref resolution: a `"$N"` reference maps to its `N:T<hexlen>,<html>` row's HTML,
  sliced by the declared **byte** length
- [x] 2.3 Implement the extractor + ref map over the flight stream

## 3. Deel adapter — Provider + Fetch (TDD)

- [x] 3.1 Test `Provider()` returns `"deel"` and the adapter is NOT boardless (board-based)
- [x] 3.2 Test `Fetch` GETs `https://jobs.deel.com/<board>` exactly once (no per-posting detail
  request) and yields the postings from the embedded payload
- [x] 3.3 Implement `deel.go`: `deel` struct over the HTML/HTTP client, `NewDeel`, the single
  board GET, parse `careerPageSettings` + `jobPostings`, resolve descriptions, map to `Job`

## 4. Field mapping (TDD)

- [x] 4.1 Test mapping of one real posting: `ExternalID` = posting `id`; `URL` =
  `https://jobs.deel.com/<board>/job-details/<id>/overview`; `Title` = `title`;
  `Company` = `careerPageSettings.preferredOrganizationName`;
  `Location` = joined `job.jobLocations[].location.name`;
  `Description` = `sanitizeHTML(<resolved richtext>)` (structure kept, active content stripped)
- [x] 4.2 Test `PostedAt` from `createdAt` via `parseRFC3339`; nil when absent/unparseable
- [x] 4.3 Test `Company` falls back to `e.Company` when `preferredOrganizationName` is absent,
  and `Remote` is set via the shared `isRemote` heuristic (e.g. a "Fully Remote" title)

## 5. Edge cases (TDD)

- [x] 5.1 Test an empty board (`deel_empty.html`) yields zero jobs and no error
- [x] 5.2 Test a page with no decodable `jobPostings` payload returns an error (loud failure,
  not a silent empty catalogue)
- [x] 5.3 Test a posting whose `id` is empty is dropped (cannot collide on the dedup key)

## 6. Registration and configuration

- [x] 6.1 Register `NewDeel(c)` in `sources.All`; confirm `reg`'s duplicate-provider guard passes
  and `deel` appears among the filterable providers (board-based, not boardless)
- [x] 6.2 Harvest live tenants into `sources/deel.yml`: seed slugs (Google `site:jobs.deel.com`
  + confirmed brands: klarna, deel, dott, dfns, airbase, mako, cardo, cablex, syone, …),
  validate each via `GET /<slug>/sitemap.xml` (XML = live tenant), keep the live ones with a
  human-readable company name per entry

## 7. Verification

- [x] 7.1 `go build ./... && go vet ./... && go test ./internal/sources/...` all green
- [x] 7.2 Focused live check: run the adapter (real client) against `klarna` and confirm real
  postings normalize (title + sanitized-HTML description + id + url + createdAt + location);
  confirm the validated-registry fail-fast still accepts `deel`
