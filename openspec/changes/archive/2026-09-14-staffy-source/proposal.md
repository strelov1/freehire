## Why

Staffy (`jobs.wearestaffy.com`) is a real recruiting-agency job board found while draining
`board_submissions` (id 193, `jobs.wearestaffy.com/positions/full-stack-software-engineer-
2006`). Initially misjudged and deleted as a low-value single-company vanity page (matching
the `huntingcube.ai` submission triaged the same pass, a genuine agency-marketing page with
no listing at all) — re-investigation found a real, substantial, entirely-static-HTML board:
`/vacantes` lists 61 live IT-heavy postings (AI/data/cloud/DevOps roles, almost entirely
Latin-American market) with no client-side rendering or API call required at all.

## What Changes

- Add a `staffy` source adapter (`internal/ingest/sources/staffy.go`). Boardless — Staffy is
  a single recruiting agency, not a multi-tenant platform other companies use, the same
  shape `lumenalta`'s existing boardless adapter already has.
- Listing: `GET https://jobs.wearestaffy.com/vacantes`, fully server-rendered static HTML.
  Every posting is an `<article class="job-card">` linking to `/positions/<slug>`; the
  page also states a declared total (`<h2 class="section-kicker-title accent">61
  activas</h2>`), verified live to equal the number of distinct `job-card` links — the
  adapter's completeness proof, since the platform has no pagination at all.
- Detail: each posting's own static HTML page carries a clean `.metadata` block of exactly
  three `<span>` elements (location, work-arrangement text, seniority label — e.g.
  `Argentina` / `Remoto` / `Junior`), plus `<h2>`-delimited prose sections (About the
  company, About the role, Responsibilities, Requirements, Nice to have) concatenated into
  the description.
- Confirmed live: the "About the company" section is NOT a reliable per-posting employer
  signal — it is absent on most postings, and where present sometimes describes Staffy
  itself ("We are a young and fast-growing recruiting company...") and sometimes an
  anonymized end-client ("It works with leading AI organizations..."), never a real,
  consistently-parseable company name. `CompanyEntry.Company` (the curator-configured
  agency name) is used directly, the same posture `recruiterflow`/`huntingcube`-shaped
  agency boards already established.
- Register `staffy` in `sources.All`. No `internal/ingest/atsboard` entry — this is a
  single company's own domain (`jobs.wearestaffy.com`), not a reusable multi-tenant
  platform pattern; the board id is fixed/implicit, matching `lumenalta`'s own boardless
  registration (no board-id parsing needed from a URL at all).
- Register the board via `cmd/add-board` once merged and deployed, closing
  `board_submissions` id 193.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `source-ingest`: add a requirement that `staffy` is a registered boardless provider —
  a static-HTML listing-enumerates/detail-hydrates adapter over `jobs.wearestaffy.com`,
  yielding the normalized job shape including a seniority level mapped from the detail
  page's own label and a location/work-mode signal from its own metadata spans.

## Impact

- New files `internal/ingest/sources/staffy.go` (+ test).
- `internal/ingest/sources/registry.go`: one new registration line.
- No `internal/ingest/atsboard` changes.
- Frontend source-facet enum (`contracts.ts`) regenerated to include `staffy`.
