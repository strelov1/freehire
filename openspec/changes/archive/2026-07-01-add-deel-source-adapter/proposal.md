## Why

Deel has launched its own multi-tenant ATS at **`jobs.deel.com/<orgSlug>`** (a Next.js app
backed by `api-prod.letsdeel.com`). It already hosts career pages for many companies — Deel
itself (~169 jobs) and Klarna (~113) down to a long tail of startups/scaleups (Dott, Dfns,
Airbase, Mako, Cardo, Syone, …) — and is growing. We do not ingest it yet. The big-name tech
ATSs (Greenhouse/Lever/Ashby) are covered; Deel's ATS is a net-new platform and a meaningful
coverage gap.

The public board page is **fully server-rendered**: each `jobs.deel.com/<slug>` inlines its
complete job payload in the Next.js flight stream (`self.__next_f.push([1,"…"])`), including
every posting's full HTML description (referenced by `$N` ids that resolve to text rows in the
same payload). So one `GET` per board assembles every Job with no per-posting detail request —
the same self-contained, embedded-payload pattern as the existing `google` adapter. No private
API, no headless browser.

## What Changes

- Add a `deel` source adapter (`internal/sources/deel.go`) speaking the existing `Source`
  interface, registered with one `NewDeel(c)` line in `sources.All`. It is a board-based
  multi-tenant adapter (NOT boardless): the configured `board` is the org's URL slug.
- **Fetch** (single request per board): `GET https://jobs.deel.com/<board>` → parse the Next.js
  flight payload embedded in the page:
  - Concatenate the JS-string bodies of every `self.__next_f.push([1,"…"])` chunk and decode
    them (JSON string unescaping) into one flight stream.
  - Read `careerPageSettings` (for `preferredOrganizationName`) and the `jobPostings[]` array
    (the postings) out of the stream.
  - Resolve each posting's `richtextDescription` `"$N"` reference to its text row
    `N:T<hexlen>,<html>` in the same stream (the `<hexlen>` is a **byte** length).
- **Field mapping** per posting:
  - `ExternalID` = the posting `id` (the UUID used in the public URL and the org sitemap).
  - `URL` = `https://jobs.deel.com/<board>/job-details/<id>/overview`.
  - `Title` = `title`.
  - `Company` = `careerPageSettings.preferredOrganizationName`, falling back to `e.Company`.
  - `Location` = the posting's `job.jobLocations[].location.name` values, joined.
  - `Description` = `sanitizeHTML(<resolved richtext HTML>)`.
  - `Remote` = the shared `isRemote` heuristic over title+location (Deel exposes **no**
    structured workplace-type field, so `WorkMode` is left empty for the location parser).
  - `PostedAt` = `createdAt` (RFC3339) via `parseRFC3339`.
- **Harvest** the tenant list into `sources/deel.yml`: seed org slugs (Google
  `site:jobs.deel.com` + the confirmed brands above), validate each against its per-org
  sitemap (`GET /<slug>/sitemap.xml` returns XML for a real tenant, an HTML shell otherwise —
  the page itself is always `200`, so the sitemap is the reliable liveness signal), and keep
  the live ones. This reuses the established `cmd/harvest-boards` board-harvest pattern.

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline and write path unchanged. -->

### Modified Capabilities
- `source-ingest`: add a requirement that `deel` is a registered provider — a board-based
  adapter that enumerates a Deel ATS org's postings from the board page's embedded Next.js
  flight payload (one request, no per-posting detail fetch), yielding the normalized job shape
  with a sanitized-HTML description resolved from the payload's `$N` text references.

## Impact

- **New code**: `internal/sources/deel.go` + `deel_test.go` + a trimmed real fixture under
  `internal/sources/testdata/`; one registration line in `sources.All`.
- **Dependencies**: none new (HTML sanitize + JSON stdlib already present).
- **Config**: one new board file (`sources/deel.yml`). No new env vars (Deel ATS is keyless).
- **Cron**: one new schedule for `sources/deel.yml` (one file per provider, crawled
  independently) — ops change, out of this repo's code scope.
- **DB**: none — reuses `UpsertJob` (`source = "deel"`, namespaced `external_id`).
- **Out of scope (known seams)**:
  - The `rest/v2` JSON API under `api-prod.letsdeel.com` exists (it answers a structured JSON
    404) but its exact public path is server-side only; the embedded-payload parse is fully
    sufficient, so the API is left as a future hardening option.
  - Structured compensation (`currentCompensation`/`isCompensationVisible`) and
    department/team/employment-type metadata are not mapped (enrichment owns salary; the Job
    shape carries none of these).
  - The flight-format parse is inherently brittle (a markup change breaks it) — by design it
    fails loudly (a decode error, never silent bad data), matching the `google` adapter's
    embedded-payload seam.
  - **Revolut** (`revolut.com/careers`) is a separate, non-Deel custom site and is handled as
    its own change, not here.
