## 1. Extend HTTPClient with GetHTML

- [x] 1.1 Add `GetHTML(ctx, url string) (*html.Node, error)` to the `HTTPClient` interface; implement it on `Client` (GET via the existing `do`, parse the body with `golang.org/x/net/html`); promote `golang.org/x/net/html` to a direct dependency (`go mod tidy`)
- [x] 1.2 Add a `GetHTML` method to the existing test fakes (`fakeHTTP`, `routedHTTP`, `gqlHTTP`) that parses a canned HTML string, so the package still compiles and the fakes can serve detail pages

## 2. Microdata extraction helpers (TDD)

- [x] 2.1 Test a helper that, given a parsed `*html.Node`, returns the text of the first element with a given `itemprop` (e.g. `title`) and "" when absent
- [x] 2.2 Test a helper that returns the rendered inner HTML of the first element with `itemprop="description"` (so it can be sanitized), and "" when absent
- [x] 2.3 Implement the two helpers over `html.Node` (depth-first walk; render children for the inner-HTML case)

## 3. SuccessFactors adapter — Provider + sitemap→detail Fetch (TDD)

- [x] 3.1 Test `Provider()` returns `"successfactors"`
- [x] 3.2 Test `Fetch` GETs `https://<board>/job_sitemap.xml`, and per `<loc>` GETs the job page, yielding the normalized jobs; assert the sitemap URL and that each `<loc>` is fetched
- [x] 3.3 Implement `successfactors.go`: `successfactors` struct over `HTTPClient`, `NewSuccessFactors`, a `sitemap` XML struct decoded via `GetXML`, then `fetchDetails(entries, workers, detail)` GET-ting each page with `GetHTML`

## 4. Field mapping (TDD)

- [x] 4.1 Test mapping: `ExternalID` = numeric id parsed from the `<loc>` path; `URL` = `<loc>`; `Title` from `itemprop="title"` (fallback `og:title`); `Company = e.Company`; `Description = sanitizeHTML(inner HTML of itemprop="description")` (active content stripped, structure kept); `Location` = ""
- [x] 4.2 Test `PostedAt` derived from the entry's `<lastmod>` (`2006-01-02`); nil when absent/unparseable
- [x] 4.3 Test `Remote` via `isRemote(title)`

## 5. Isolation and empty-board behavior (TDD)

- [x] 5.1 Test a failed job-page fetch for one `<loc>` drops only that posting and still yields the rest (no board abort)
- [x] 5.2 Test an empty sitemap (no `<url>` entries) yields zero jobs and no error

## 6. Registration and configuration

- [x] 6.1 Register `NewSuccessFactors(c)` in `sources.All`; confirm `reg`'s duplicate-provider guard still passes
- [x] 6.2 Add at least one verified `successfactors` entry to `sources/successfactors.yml` (`board: <career-site-host>`), validated live (>0 jobs in its sitemap)

## 7. Verification

- [x] 7.1 `go build ./... && go vet ./... && go test ./internal/sources/...` all green
- [x] 7.2 Focused live check: run the adapter (real `Client`) against the configured board and confirm real postings normalize (title + sanitized description + id + url + lastmod date); confirm the validated-registry fail-fast still accepts `successfactors`
