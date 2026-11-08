## Why

Teamtailor powers the career sites of a large share of European (Nordic/DACH) employers
(e.g. Tibber at jobs.tibber.com, Normative at careers.normative.io). It is one of four
OpenJobs-covered ATS platforms we do not yet ingest, and the one with the widest EU reach —
adding it brings that segment into the pool. Its boards are JS-rendered, but the listing page
server-renders job cards and every job page embeds a schema.org JobPosting `ld+json` block,
so the postings are reachable with simple GET requests, no browser or bot-wall.

## What Changes

- Add a `teamtailor` source adapter (`internal/sources/teamtailor.go`) speaking the existing
  `Source` interface, registered with one `NewTeamtailor(c)` line in `sources.All`.
- It follows the established **list → detail** pattern: enumerate jobs from the career
  site's `GET https://<board>/jobs` HTML (each job card is an anchor to `/jobs/<id>-<slug>`),
  paginating `?page=N` until a page yields zero job links, then GET each job page and extract
  the posting from its schema.org JobPosting `application/ld+json` block, fanned out with the
  shared `fetchDetails` bounded-concurrency helper.
- The `sources.yml` `board` value is the **career-site host** (e.g. `jobs.tibber.com`), as
  with the `successfactors` adapter; the adapter builds `https://<board>/jobs` for the listing
  and GETs each absolute job URL for detail. The host is explicit and does not depend on the
  `<slug>.teamtailor.com` alias staying live.
- The listing source is the **HTML page, not the sitemap**: observed boards have job-less or
  stale sitemaps (Normative's `sitemap.xml` lists no jobs; Tibber's lags the listing), so the
  server-rendered listing is the only complete, fresh enumeration.
- Detail fields come from the JobPosting `ld+json`: `title`, `description` (double HTML-encoded
  → `html.UnescapeString` then `sanitizeHTML`, as the Greenhouse adapter does), `datePosted`
  (RFC3339 → `posted_at`), and `jobLocation`'s `addressLocality`/`addressCountry` → `location`.
  `external_id` is the numeric id from the job URL path. No new HTTP-client method is needed —
  `GetHTML`, `sanitizeHTML`, `fetchDetails`, and `parseRFC3339` already exist.
- **Remote** is inferred from the JobPosting `jobLocationType == "TELECOMMUTE"`, falling back
  to the shared `isRemote` heuristic over the location/title.

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline and write path unchanged. -->

### Modified Capabilities

- `source-ingest`: add a requirement that `teamtailor` is a registered provider — a
  listing-HTML-enumerated, JSON-LD detail adapter yielding the normalized job shape with a
  sanitized-HTML description, consistent with the existing detail-fetching adapters.

## Impact

- **New code**: `internal/sources/teamtailor.go` + `teamtailor_test.go`; one registration
  line in `sources.All`.
- **Dependencies**: none new — reuses the `GetHTML` transport and HTML/JSON helpers already
  added for `successfactors`.
- **Config**: one new `sources.yml` file (`sources/teamtailor.yml`). No new env vars.
- **DB**: none — reuses `UpsertJob` (`source = "teamtailor"`, namespaced `external_id`).
- **Out of scope (known seams)**: department/team parsing (left to enrichment); boards that
  disable the public listing page (none observed; would need the authenticated JSON:API);
  the remaining three OpenJobs platforms (Join.com, Breezy, Jobvite), added as their own
  changes one at a time.
