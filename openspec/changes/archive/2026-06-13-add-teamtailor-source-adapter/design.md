## Context

`internal/sources` holds one adapter per ATS behind the `Source` interface; several
(smartrecruiters, rippling, bamboohr, gem, successfactors) use the shared `fetchDetails`
bounded pool when the list lacks the description. The `successfactors` adapter already added
`GetHTML(ctx, url) (*html.Node, error)` to `HTTPClient` and the `sanitizeHTML` helper, so the
HTML-fetch transport Teamtailor needs is in place. Teamtailor is the first adapter whose detail
is a JSON-LD block embedded in HTML (rather than schema.org microdata), so it adds a small
`ld+json` extraction step but no new transport.

Contract confirmed live against `jobs.tibber.com` and `careers.normative.io`:

- `GET https://<host>/jobs` → server-rendered HTML; each job card is an `<a href>` to an
  absolute `https://<host>/jobs/<numeric-id>-<slug>` URL. Pagination is `?page=N`; a page past
  the last yields zero job anchors (Tibber: 15 jobs on page 1, page 2 empty).
- The sitemap is NOT reliable for enumeration: Normative's `sitemap.xml` lists no job URLs at
  all, and Tibber's lags the listing (11 vs 15). The listing HTML is the only complete source.
- `GET <job-url>` → HTML containing a `<script type="application/ld+json">` with
  `"@type":"JobPosting"`: `title`, `description` (double HTML-encoded — `&lt;p&gt;…`),
  `datePosted` (RFC3339, e.g. `2026-06-08T00:00:00+02:00`), `employmentType`, and a
  `jobLocation` with `address.addressLocality` / `address.addressCountry`.

## Goals / Non-Goals

**Goals:**

- A `teamtailor` adapter that enumerates a board's listing pages and yields normalized jobs
  with sanitized-HTML descriptions, reusing the list→detail pattern and existing helpers.
- A small, reusable JobPosting `ld+json` extractor over `html.Node`.

**Non-Goals:**

- Department/team extraction (left to enrichment).
- The authenticated Teamtailor JSON:API (`api.*.teamtailor.com`, needs a key).
- Sitemap-based enumeration (rejected — incomplete/stale, see Context).

## Decisions

- **Board = career-site host.** `e.Board` is the host (e.g. `jobs.tibber.com`); the adapter
  builds `https://<board>/jobs` for the listing. Explicit and independent of the
  `<slug>.teamtailor.com` alias. Mirrors the `successfactors` board convention.
- **Enumerate via the listing HTML, paginating until empty.** `Fetch` GETs
  `https://<board>/jobs?page=N` for N = 1, 2, … (`GetHTML`), collecting job-card hrefs from
  each page, and stops at the first page that yields zero job links. A safety cap (e.g. 100
  pages) bounds a pathological loop. The collected URLs feed `fetchDetails(urls, workers, …)`,
  which GETs each job page and maps it; a failed page drops only that posting.
- **Job-link collection** walks the parsed tree for `<a>` elements whose `href` carries a
  parseable `/jobs/<digits>` id, de-duplicated (a card may link the title and a button to the
  same job), preserving first-seen order. Each href is resolved against the listing URL via
  `url.ResolveReference`, so a board emitting relative hrefs still yields fetchable absolute
  URLs (an absolute href resolves to itself) — without this, a relative href would fail the
  detail GET on a bare path and silently drop the posting.
- **JobPosting `ld+json` extraction** walks the tree for the first
  `<script type="application/ld+json">` whose decoded JSON has `"@type":"JobPosting"`, and
  returns the decoded struct. Decoding is into a typed struct (title, description, datePosted,
  employmentType, jobLocation.address.{addressLocality,addressCountry}, optional
  jobLocationType). A page without such a block drops that posting (ok=false).
- **Job mapping:**
  - `ExternalID` = the leading numeric segment of the job URL path (`/jobs/<id>-…`); the
    pipeline namespaces it by board.
  - `URL` = the absolute job URL from the listing.
  - `Title` = JobPosting `title`; `Company` = `e.Company`.
  - `Description` = `sanitizeHTML(html.UnescapeString(description))` (double-encoded → unescape
    once, then sanitize), exactly as the Greenhouse adapter handles its `content`.
  - `Location` = `joinNonEmpty(addressLocality, addressCountry)`.
  - `Remote` = `jobLocationType == "TELECOMMUTE"` OR `isRemote(joinNonEmpty(location, title))`.
  - `PostedAt` = `parseRFC3339(datePosted)`.

## Risks / Trade-offs

- **HTML scraping is more fragile than JSON.** Mitigation: enumeration relies only on the
  `/jobs/<id>` URL shape (stable, it is the public permalink), and detail relies on the
  schema.org JobPosting `ld+json` (a standard Teamtailor emits for SEO), not on CSS classes.
  A missing field yields an empty value and the posting still ingests. Covered by table-driven
  tests over canned HTML, no live network in unit tests.
- **Pagination correctness.** The "stop on first empty page" rule plus the page cap avoids
  both under-fetching (large boards) and an unbounded loop. A board that returns the same page
  for any `?page=N` would be caught by de-duplication producing no *new* links → treated as
  empty and stopped. Covered by a multi-page test fake.
- **Per-job detail fetches** — one GET per job under the bounded pool, like other
  detail-fetching adapters; acceptable for a scheduled crawl.
- **Listing dependence** — a board that disables the public `/jobs` page is unsupported; none
  observed. That would require the authenticated JSON:API (a separate credentials decision).
