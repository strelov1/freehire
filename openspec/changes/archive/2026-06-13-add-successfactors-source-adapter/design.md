## Context

`internal/sources` holds one adapter per ATS behind the `Source` interface, with several
(smartrecruiters, rippling, bamboohr, gem) using the shared `fetchDetails` bounded pool when
the list lacks the description. The shared `HTTPClient` exposes `GetJSON`, `GetXML`,
`PostJSON` — all decode structured bodies. SuccessFactors is the first source whose detail is
HTML, so the interface must grow a raw-HTML fetch.

Contract confirmed live against `jobs.tetrapak.com`:

- `GET https://<host>/job_sitemap.xml` → `<urlset>` of `<url>{<loc>, <lastmod>, …}` — 256
  job URLs of the form `/job/<slug>/<numeric-id>/`, each with a `<lastmod>` date. Plain XML,
  no auth, no Cloudflare.
- `GET <loc>` → server-rendered HTML with an `itemtype="http://schema.org/JobPosting"` scope
  exposing `itemprop="title"` and `itemprop="description"` (the description's inner HTML is
  the full job body). Location/date are NOT in the microdata.
- The JS search widget (`xweb/rmk-jobs-search`, backed by `performancemanager.successfactors.eu`)
  and `tile-search-results` are runtime-constructed and return nothing useful to a plain
  client — the sitemap is the clean enumeration path instead.

## Goals / Non-Goals

**Goals:**

- A `successfactors` adapter that enumerates a site's sitemap and yields normalized jobs with
  sanitized-HTML descriptions, reusing the list→detail pattern and helpers.
- A reusable `GetHTML` on `HTTPClient` so future HTML-scraping adapters share one transport.

**Non-Goals:**

- Structured location/department extraction (left empty → enrichment fills it).
- Locale negotiation — read the site's default-locale pages.
- The JS widget / `tile-search-results` API.

## Decisions

- **Enumerate via the sitemap, detail via the job page.** `Fetch` GETs `job_sitemap.xml`
  (decoded with the existing `GetXML`), then `fetchDetails(entries, workers, …)` GETs each
  job page (`GetHTML`) and maps it. A failed page drops only that posting.
- **Extend `HTTPClient` with `GetHTML(ctx, url) (*html.Node, error)`** using
  `golang.org/x/net/html` (already transitive via bluemonday; promote to direct). The real
  `Client` adds it alongside `GetJSON`; the test fakes (`fakeHTTP`, `routedHTTP`, and the
  gem `gqlHTTP`) gain a `GetHTML` that parses a canned HTML string. This is the seam for all
  HTML-rendered ATS, not a SuccessFactors-only shim.
- **Microdata extraction** walks the parsed tree for the first element with
  `itemprop="title"` (text) and `itemprop="description"` (rendered inner HTML), via small
  helpers over `html.Node`. Title falls back to the `og:title` meta if microdata is absent.
- **Job mapping:**
  - `ExternalID` = the numeric id parsed from the `<loc>` path (pipeline namespaces it).
  - `URL` = the `<loc>`.
  - `Title` = `itemprop="title"` (fallback `og:title`); `Company` = `e.Company`.
  - `Location` = "" (enrichment derives it).
  - `Description` = `sanitizeHTML(<inner HTML of itemprop="description">)`.
  - `Remote` = `isRemote(title)` (location is empty; enrichment refines remote).
  - `PostedAt` = the entry's `<lastmod>` via the existing `parseDate` ("2006-01-02").

## Risks / Trade-offs

- **HTML scraping is more fragile than JSON.** Mitigation: rely on schema.org microdata
  (`itemprop`), the most stable hook SuccessFactors exposes, not on CSS classes or CSB
  template tokens. A missing field yields an empty value, and the posting still ingests.
  Covered by table-driven tests over canned HTML, no live network in unit tests.
- **Per-job detail fetches** — a large site (256 here) issues one GET per job under the
  bounded pool, like other detail-fetching adapters; acceptable for a scheduled crawl.
- **Empty location** weakens filtering until enrichment runs; accepted deliberately over a
  fragile heuristic (decided with the user).
- **Sitemap dependence** — a site that disables `job_sitemap.xml` is unsupported; none
  observed. That would require the JS search API (a separate fetch-infra decision).
