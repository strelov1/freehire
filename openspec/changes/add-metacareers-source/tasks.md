## 1. De-risk the transport — live uTLS spike (BEFORE any adapter code)

- [ ] 1.1 Add `github.com/refraction-networking/utls` to the module (`go get`, `go mod tidy`)
- [ ] 1.2 Write a throwaway spike (a `cmd/` `main` or a `//go:build ignore`/manual test) that GETs `https://www.metacareers.com/jobsearch/sitemap.xml` and one `job_details/<id>` page through an `http.Client` whose transport does the TLS handshake with `utls.UClient(..., utls.HelloChrome_Auto)`, selecting HTTP/2 vs HTTP/1.1 from the negotiated ALPN. Run it **from a host whose egress matches prod** and assert: sitemap returns `200` with `<loc>` entries, and a detail page returns `200` with an `application/ld+json` `JobPosting`. If it `400`s, stop and report (Meta also fingerprints h2 → re-evaluate approach) before building the adapter.
- [ ] 1.3 While the spike is live, resolve the completeness seam: count the sitemap entries and check for a sitemap index / shards (or a master `sitemap.xml.gz` that lists more job shards). Record whether 603 is the full catalogue or a cap.

## 2. Chrome-fingerprint client over the guarded dialer (TDD)

- [ ] 2.1 Test that the SSRF guard is preserved: the Chrome-fingerprint client refuses a loopback/internal target (mirror `safehttp`'s loopback-refusal test), proving the uTLS path still dials through the guarded `Control` hook
- [ ] 2.2 Implement the Chrome-fingerprint transport (uTLS `HelloChrome_Auto` handshake layered over `safehttp`'s guarded dialer, with ALPN→HTTP/2-or-1.1 selection) and a `sources` constructor that wraps it in the existing `Client` shape (same `do`/retry/limit/userAgent), exposing the ordinary `XMLGetter`/`HTMLGetter` roles
- [ ] 2.3 `go build ./... && go vet ./...` green

## 3. Meta adapter — Provider + sitemap→detail Fetch (TDD)

- [ ] 3.1 Test `Provider()` returns `"meta"`
- [ ] 3.2 Test `Fetch` GETs `https://www.metacareers.com/jobsearch/sitemap.xml`, and per `<loc>` GETs the job page, yielding the normalized jobs; assert the sitemap URL and that each `<loc>` is fetched (fake `XMLGetter`+`HTMLGetter`, no real network, mirroring `successfactors_test.go`)
- [ ] 3.3 Implement `metacareers.go`: `metacareers` struct over an `XMLGetter`+`HTMLGetter` interface, `NewMetaCareers`, a sitemap XML struct decoded via `GetXML`, then `fetchDetails(entries, workers, detail)` GET-ting each page with `GetHTML` and decoding the ld+json via the shared `ldJobPosting()` helper. Boardless: `Fetch` ignores `e.Board`, uses `e.Company`

## 4. Field mapping (TDD)

- [ ] 4.1 Test mapping: `ExternalID` = numeric id parsed from the `/job_details/<id>/` `<loc>` path; `URL` = `<loc>`; `Title` from ld+json `title`; `Company = e.Company` (`"Meta"`); `Description = sanitizeHTML(ld+json description)` (active content stripped, structure kept)
- [ ] 4.2 Test `Location` = first `jobLocation[].name`, and that the broken `jobLocation[].address.*` fields are NOT used (a fixture with `address` locality `"Aiken, SC"` but `name` `"Menlo Park, CA"` must yield `"Menlo Park, CA"`); `""` when no `jobLocation`
- [ ] 4.3 Test `PostedAt` from ld+json `datePosted` (RFC3339), falling back to the entry's `<lastmod>`, nil when both absent/unparseable; and `Remote` via `isRemote`

## 5. Isolation and empty-board behavior (TDD)

- [ ] 5.1 Test a failed job-page fetch for one `<loc>` drops only that posting and still yields the rest (no board abort)
- [ ] 5.2 Test an empty sitemap (no `<url>` entries) yields zero jobs and no error

## 6. Registration and configuration

- [ ] 6.1 Register `NewMetaCareers(<chrome-fingerprint client>)` in `sources.All`, building the Chrome-fingerprint client once and passing it ONLY to meta; confirm `reg`'s duplicate-provider guard still passes and the other adapters keep the shared client
- [ ] 6.2 Add the boardless `company: Meta` / `provider: meta` entry to `sources/custom.yml`

## 7. Verification

- [ ] 7.1 `go build ./... && go vet ./... && go test ./internal/sources/... ./internal/safehttp/...` all green
- [ ] 7.2 Focused live check: run the real adapter (Chrome-fingerprint `Client`) against Meta and confirm real postings normalize (title + sanitized description + id + url + location + date); confirm the validated-registry fail-fast still accepts `meta`
