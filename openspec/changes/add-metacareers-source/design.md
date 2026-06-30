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
- A headless-browser tier (uTLS is the pure-Go fit for sitemap + ld+json).
- Enumerating postings Meta itself does not list in its sitemap.
- Structured multi-location modeling (take the first location; enrichment refines).

## Decisions

- **uTLS transport scoped to Meta, over the guarded dialer.** Add a constructor that builds an
  `*http.Client` whose transport performs the TLS handshake with
  `utls.UClient(conn, cfg, utls.HelloChrome_Auto)` instead of `crypto/tls`, while the underlying
  TCP dial still goes through `safehttp`'s `Control`-guarded dialer (SSRF protection on the
  original request and every redirect hop). Wrap that `*http.Client` in the existing
  `sources.Client` shape (same retry/limit/`do` machinery, same `userAgent`) so the adapter
  consumes the ordinary `XMLGetter`/`HTMLGetter` roles and nothing else in the package learns
  about uTLS. `sources.All` constructs this client once and passes it **only** to
  `NewMetaCareers(c)`.
  - The handshake must negotiate ALPN the way Chrome does. `HelloChrome_Auto` advertises `h2`;
    after the handshake the transport selects HTTP/2 vs HTTP/1.1 from the negotiated protocol.
    This is the fiddly part and is **de-risked by Task 1 (a live spike) before any adapter code**.
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

- **HTTP/2 fingerprinting beyond TLS.** Meta's edge might also fingerprint the h2 layer.
  Mitigation: Task 1 is a standalone live spike (a tiny `main` or test hitting
  `metacareers.com` through the uTLS client and asserting `200` + real ld+json) — built and run
  **before** the adapter, so a dead end is found in one task, not after the whole adapter.
- **New third-party dependency.** `refraction-networking/utls` is a widely-used, pure-Go,
  actively-maintained TLS library; no cgo, no browser. Scope is contained to one client
  constructor.
- **Fingerprint drift.** `HelloChrome_Auto` tracks a recent Chrome; Meta could tighten its edge
  and require a newer hello. Mitigation: the hello id is a one-line bump; the failure mode is a
  clean `400` (the existing per-board isolation counts it as a failed board, never corrupts data).
- **HTML/ld+json scraping is more fragile than a JSON API.** Mitigation: rely on schema.org
  `JobPosting` (the most stable hook), reuse the shared `ldJobPosting`, and cover mapping with
  table-driven tests over canned HTML — no live network in unit tests.
- **Sitemap completeness (603 entries).** May be a recent-window cap. Mitigation: Task 1/2 check
  for a sitemap index or shards; if 603 is all Meta lists, that is the catalogue and is accepted.
- **SSRF guard preserved.** The uTLS path reuses `safehttp`'s guarded dialer, so the new client is
  not an SSRF regression despite bypassing the standard TLS stack.
