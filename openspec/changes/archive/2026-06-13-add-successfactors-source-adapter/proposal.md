## Why

SAP SuccessFactors powers the career sites of a large share of enterprise employers
(e.g. Tetra Pak at jobs.tetrapak.com). Those sites are JS/widget-rendered, but each one
publishes a plain-XML job sitemap and server-renders every job page with schema.org
JobPosting microdata — so the postings are reachable with simple GET requests, no browser
or bot-wall. Adding a `successfactors` adapter brings this large segment into the pool.

## What Changes

- Add a `successfactors` source adapter (`internal/sources/successfactors.go`) speaking the
  existing `Source` interface, registered with one `NewSuccessFactors(c)` line in `sources.All`.
- It follows the established **list → detail** pattern: enumerate jobs from the site's
  `GET /job_sitemap.xml` (each `<url>` carries the job `<loc>` and a `<lastmod>` date),
  then GET each job page and extract the title + description from its schema.org JobPosting
  microdata, fanned out with the shared `fetchDetails` bounded-concurrency helper.
- The `sources.yml` `board` value is the **career-site host** (e.g. `jobs.tetrapak.com`);
  the adapter builds `https://<board>/job_sitemap.xml` and GETs each `<loc>` for detail.
- **Extend the shared `HTTPClient` interface with `GetHTML`** (fetch a URL and return its
  parsed HTML tree via `golang.org/x/net/html`, already in the module graph). SuccessFactors
  is the first HTML-scraping adapter; this is the reusable seam for future HTML-rendered
  ATS, rather than a one-off shim. The real `Client` implements it; the test fakes gain it.
- **Location is left empty on purpose** — SuccessFactors does not expose it in the
  microdata, and AI enrichment already derives structured fields (including location) from
  the description. No fragile URL-slug or template-token heuristic.

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline and write path unchanged. -->

### Modified Capabilities

- `source-ingest`: add a requirement that `successfactors` is a registered provider — a
  sitemap-enumerated, HTML-microdata detail adapter yielding the normalized job shape with a
  sanitized-HTML description, consistent with the existing detail-fetching adapters.

## Impact

- **New code**: `internal/sources/successfactors.go` + `successfactors_test.go`; one
  registration line in `sources.All`; a `GetHTML` method on `HTTPClient`/`Client` and the
  test fakes.
- **Dependencies**: promote `golang.org/x/net/html` to a direct dependency (already present
  transitively via bluemonday). No new third-party code.
- **Config**: one new `sources.yml` entry (`sources/successfactors.yml`). No new env vars.
- **DB**: none — reuses `UpsertJob` (`source = "successfactors"`, namespaced `external_id`).
- **Out of scope (known seams)**: structured location/department parsing from CSB template
  tokens (left to enrichment); locale handling (the adapter reads the default-locale pages);
  sites that disable the job sitemap (none observed; would need the JS search API).
