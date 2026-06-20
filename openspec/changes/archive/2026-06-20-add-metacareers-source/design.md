## Context

`internal/sources` holds one adapter per ATS behind the `Source` interface. The shared
`HTTPClient` already exposes `GetXML` (sitemap) and `GetHTML` (parsed HTML tree), and
`ldJobPosting()` decodes the first `application/ld+json` `JobPosting` on a page — so the
`successfactors`/`teamtailor`/`breezy` list→detail pattern and the `fetchDetails` bounded pool
are reusable as-is. The one thing Meta needs that no existing source needs is a **non-default TLS
fingerprint**: the shared `Client` is built on `safehttp.NewClient`, whose transport presents
Go's standard ClientHello, which Meta's edge rejects.

Contract confirmed live against `www.metacareers.com` (real Chrome from the prod host, and the
diagnosis below):

- `GET https://www.metacareers.com/jobsearch/sitemap.xml` → `<urlset>` of
  `<url>{<loc>, <lastmod>}` — flat list of `https://www.metacareers.com/profile/job_details/<id>/`
  URLs (603 in the observed shard). Plain XML.
- `GET <loc>` → server-rendered HTML carrying `<script type="application/ld+json">` with
  `@type: JobPosting`: `title`, `description` (HTML), `responsibilities`, `qualifications`,
  `datePosted`, `validThrough`, `employmentType`, and `jobLocation[]` (each a `Place` with a
  correct `name` like `"Menlo Park, CA"`).
- **TLS diagnosis:** Go `net/http` and curl get `HTTP 400` + `proxy-status: http_request_error`
  on *every* path including `/`; a real Chrome on the same host gets `200`. ⇒ JA3/TLS-fingerprint
  block at Meta's edge, not IP, not persisted-query.
- **Data bug to avoid:** inside each `jobLocation[].address`, `addressLocality`/`addressRegion`/
  `addressCountry.name` are wrong (every location repeats `"Aiken, SC"` / a `"USA"` array). Only
  `jobLocation[].name` is reliable.

## Goals / Non-Goals

**Goals:**
- A boardless `meta` adapter that enumerates the jobsearch sitemap and yields normalized jobs
  with sanitized-HTML descriptions, reusing the list→detail pattern and helpers.
- A Chrome-fingerprint (uTLS) HTTP client, scoped to this adapter, that still routes through the
  `safehttp` SSRF-guarded dialer.

**Non-Goals:**
- Changing the shared client's fingerprint or any other adapter's transport.
- A headless-browser tier (`tls-client` is the pure-Go fit for sitemap + ld+json).
- Enumerating postings Meta itself does not list in its sitemap.
- Structured multi-location modeling (take the first location; enrichment refines).

## Decisions

- **`tls-client` transport scoped to Meta, over the guarded dialer.** The live spike (Task 1)
  settled this: plain `utls` (JA3 only) `400`s on both h2 and h1 because Meta also fingerprints the
  HTTP/2 layer; `github.com/bogdanfinn/tls-client` with profile `Chrome_133` — which spoofs JA3
  **and** the Chrome h2 fingerprint via its bundled `fhttp` net/http fork — returns `200`. Build a
  `tls_client.HttpClient` with `WithClientProfile(profiles.Chrome_133)`, a timeout, and
  `WithDialer(*safehttp.GuardedDialer(timeout))` so the SSRF `Control` guard still fires after DNS
  resolution. Wrap it in a small `metacareers`-local type implementing the `XMLGetter`/`HTMLGetter`
  roles: each request is built with the Chrome header set plus `fhttp.HeaderOrderKey` (the header
  order is part of the fingerprint), the response is bounded-read, then XML-decoded or HTML-parsed.
  This wrapper is the **only** place in the package that imports `tls-client`/`fhttp`. `sources.All`
  constructs it once and passes it **only** to `NewMetaCareers(c)`.
- **Boardless single-company source.** `Provider()` returns `"meta"`; `Fetch` ignores `e.Board`
  and uses `e.Company` (`"Meta"`), mirroring `google`/`amazon`. One `sources/custom.yml` entry.
- **Enumerate via the sitemap, detail via the job page.** `Fetch` GETs the jobsearch sitemap
  (`GetXML`), then `fetchDetails(entries, workers, detail)` GETs each `job_details` page
  (`GetHTML`) and maps its ld+json. A failed page drops only that posting.
- **ld+json extraction** reuses the shared `ldJobPosting(root, &v)` helper with a struct selecting
  just the fields Meta exposes.
- **Job mapping:**
  - `ExternalID` = the numeric id parsed from the `/job_details/<id>/` `<loc>` path.
  - `URL` = the `<loc>`.
  - `Title` = ld+json `title`; `Company` = `e.Company` (`"Meta"`).
  - `Location` = first `jobLocation[].name` (`""` if none); never the broken `address.*`.
  - `Description` = `sanitizeHTML(ld+json description)`.
  - `Remote` = `isRemote(title + " " + location)`.
  - `PostedAt` = ld+json `datePosted` (RFC3339); fall back to the entry's `<lastmod>`; nil if
    both absent/unparseable.

## Risks / Trade-offs

- **HTTP/2 fingerprinting beyond TLS — confirmed, not just a risk.** The spike proved Meta
  fingerprints the h2 layer, so plain `utls` is out. `tls-client` (JA3 + Chrome h2) is the chosen
  transport because the live spike returned `200`.
- **Heavier third-party dependency (`fhttp` net/http fork).** `bogdanfinn/tls-client` bundles
  `bogdanfinn/fhttp`, a fork of the standard `net/http`, plus a `utls` fork. The fork is confined
  to the single Meta transport wrapper (it never escapes into the rest of `internal/sources`, which
  keeps stdlib `net/http`), so the blast radius is one file. Accepted by the user over a
  headless-browser tier (decided in brainstorming) because it is pure Go and reusable for other
  fingerprint-blocked sources.
- **Fingerprint drift.** `Chrome_133` tracks a specific Chrome; Meta could tighten its edge and
  require a newer profile. Mitigation: the profile is a one-line bump; the failure mode is a clean
  `400` (the existing per-board isolation counts it as a failed board, never corrupts data).
- **HTML/ld+json scraping is more fragile than a JSON API.** Mitigation: rely on schema.org
  `JobPosting` (the most stable hook), reuse the shared `ldJobPosting`, and cover mapping with
  table-driven tests over canned HTML — no live network in unit tests.
- **Sitemap completeness (603 entries).** May be a recent-window cap. Mitigation: Task 1/2 check
  for a sitemap index or shards; if 603 is all Meta lists, that is the catalogue and is accepted.
- **SSRF guard preserved.** The uTLS path reuses `safehttp`'s guarded dialer, so the new client is
  not an SSRF regression despite bypassing the standard TLS stack.
