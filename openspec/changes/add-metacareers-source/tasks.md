## 1. De-risk the transport — live fingerprint spike (BEFORE any adapter code) — DONE

- [x] 1.1 Spiked `github.com/refraction-networking/utls` (`HelloChrome_Auto`) over both HTTP/2 and HTTP/1.1 → **both `400`**. Plain JA3 spoofing is insufficient: Meta's edge fingerprints the HTTP/2 layer too (Go's h2 framer gives it away).
- [x] 1.2 Spiked `github.com/bogdanfinn/tls-client` (profile `Chrome_133`, Chrome header set + `HeaderOrderKey`) → **`200 OK`** on the jobsearch sitemap (603 `<loc>`) and on a `job_details` page (`application/ld+json` `JobPosting` present). This is the production transport. ⇒ design/proposal updated: dependency is `bogdanfinn/tls-client` (bundles the `bogdanfinn/fhttp` net/http fork + a `utls` fork), NOT `refraction-networking/utls`.
- [x] 1.3 Completeness seam resolved: `jobsearch/sitemap.xml` is a **flat** url list of **603** entries, **no `<sitemap>` shard index**. That is the catalogue this source exposes; accepted (no pagination to chase).

## 2. Chrome-fingerprint transport over the guarded dialer (TDD)

- [x] 2.1 Add an exported `GuardedDialer(timeout) *net.Dialer` to `internal/safehttp` (reusing the existing private guard `Control` hook) so the Meta client can dial through the SSRF guard via `tls_client.WithDialer`. Test it refuses a loopback/internal target (mirror the existing `safehttp` loopback-refusal test)
- [x] 2.2 Implement a Meta-only transport in `internal/sources`: a `tls_client.HttpClient` built with profile `Chrome_133` + `WithDialer(*safehttp.GuardedDialer(...))` + a timeout, wrapped in a small type that implements the `XMLGetter` and `HTMLGetter` roles (build each request with the Chrome header set + `fhttp.HeaderOrderKey`, decode XML / parse HTML with `golang.org/x/net/html`, bound the response read). A non-2xx is an error; this wrapper is the only place in the package that touches `fhttp`/`tls-client`
- [x] 2.3 Remove the now-unused `github.com/refraction-networking/utls` dependency and the throwaway spike commands; `go mod tidy`; `go build ./... && go vet ./...` green

## 3. Meta adapter — Provider + sitemap→detail Fetch (TDD)

- [x] 3.1 Test `Provider()` returns `"meta"`
- [x] 3.2 Test `Fetch` GETs `https://www.metacareers.com/jobsearch/sitemap.xml`, and per `<loc>` GETs the job page, yielding the normalized jobs; assert the sitemap URL and that each `<loc>` is fetched (fake `XMLGetter`+`HTMLGetter`, no real network, mirroring `successfactors_test.go`)
- [x] 3.3 Implement `metacareers.go`: `metacareers` struct over an `XMLGetter`+`HTMLGetter` interface, `NewMetaCareers`, a sitemap XML struct decoded via `GetXML`, then `fetchDetails(entries, workers, detail)` GET-ting each page with `GetHTML` and decoding the ld+json via the shared `ldJobPosting()` helper. Boardless: `Fetch` ignores `e.Board`, uses `e.Company`

## 4. Field mapping (TDD)

- [x] 4.1 Test mapping: `ExternalID` = numeric id parsed from the `/job_details/<id>/` `<loc>` path; `URL` = `<loc>`; `Title` from ld+json `title`; `Company = e.Company` (`"Meta"`); `Description = sanitizeHTML(ld+json description)` (active content stripped, structure kept)
- [x] 4.2 Test `Location` = first `jobLocation[].name`, and that the broken `jobLocation[].address.*` fields are NOT used (a fixture with `address` locality `"Aiken, SC"` but `name` `"Menlo Park, CA"` must yield `"Menlo Park, CA"`); `""` when no `jobLocation`
- [x] 4.3 Test `PostedAt` from ld+json `datePosted` (RFC3339), falling back to the entry's `<lastmod>`, nil when both absent/unparseable; and `Remote` via `isRemote`

## 5. Isolation and empty-board behavior (TDD)

- [x] 5.1 Test a failed job-page fetch for one `<loc>` drops only that posting and still yields the rest (no board abort)
- [x] 5.2 Test an empty sitemap (no `<url>` entries) yields zero jobs and no error

## 6. Registration and configuration

- [x] 6.1 Register `NewMetaCareers(<chrome-fingerprint client>)` in `sources.All`, building the Chrome-fingerprint client once and passing it ONLY to meta; confirm `reg`'s duplicate-provider guard still passes and the other adapters keep the shared client
- [x] 6.2 Add the boardless `company: Meta` / `provider: meta` entry to `sources/custom.yml`

## 7. Verification

- [x] 7.1 `go build ./... && go vet ./... && go test ./internal/sources/... ./internal/safehttp/...` all green
- [x] 7.2 Focused live check: run the real adapter (Chrome-fingerprint `Client`) against Meta and confirm real postings normalize (title + sanitized description + id + url + location + date); confirm the validated-registry fail-fast still accepts `meta`
