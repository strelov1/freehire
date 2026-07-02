## Why

careers-page.com is a hosted ATS that puts each client's career page on a
`<tenant>.careers-page.com` subdomain. Every tenant server-renders its job
listing as HTML and exposes a clean schema.org `JobPosting` `ld+json` block on
each job detail page — all keyless. We do not ingest it today, so live vacancies
on these boards (e.g. David Joseph & Company's founding-engineer roles) are
missing from the catalogue. A spike VALIDATED that the listing and detail pages
are keyless and machine-readable.

## What Changes

- Add a `careerspage` source adapter (`internal/sources/careerspage.go`) over the
  keyless `https://<board>.careers-page.com` career site. The board id is the
  tenant subdomain; the adapter is board-based (not boardless) and stays in the
  source facet.
- The adapter enumerates postings from the **server-rendered listing HTML**
  (`?page=N`), following pagination until a page yields no new job links, then
  fetches each job's detail page and maps its `application/ld+json` `JobPosting`
  (title, description, `datePosted`, `employmentType`, `jobLocation`,
  `hiringOrganization`) to a `Job` — reusing the existing `jsonld.go`/`html.go`
  helpers, the same shape as the icims/freshteam/successfactors adapters.
- The detail fan-out runs under the shared bounded worker pool (`fetchDetails`),
  so one failed detail request drops only that posting.
- Register the adapter in `sources.All` (one line).
- Add `sources/careerspage.yml` seeded with the validated David Joseph & Company
  board (`davidjoseph-co`).

## Capabilities

### New Capabilities
- `careerspage-source`: crawling one careers-page.com tenant's server-rendered
  job listing into the catalogue, mapping each posting's schema.org `JobPosting`
  detail into a normalized `Job`.

### Modified Capabilities
<!-- none: no existing spec's requirements change -->

## Impact

- New file `internal/sources/careerspage.go` (+ test) and one line in
  `internal/sources/source.go` (`All`).
- New board file `sources/careerspage.yml`; a cron schedule for it is an ops
  follow-up (mirrors every other per-provider board file).
- No schema, API, or migration changes; read-only over public HTTP.
