## Why

HERP (`herp.careers`) is a Japanese multi-tenant ATS with ~970 companies in an external
inventory (kalil0321/ats-scrapers' `ats-companies/herp.csv`) freehire currently cannot crawl at
all — there is no adapter, no board recognition, and no catalog entries. Live investigation
confirms it fits the project's existing adapter shape cleanly: every job posting page carries a
standard schema.org `JobPosting` `application/ld+json` block, the same structured detail source
`breezy` and `teamtailor` already read via the shared `ldJobPosting` helper — no bespoke
HTML-scraping of free text is needed, only link discovery for the listing.

## What Changes

- Add a `herp` source adapter (`internal/ingest/sources/herp.go`) speaking the existing
  `Source` interface, registered in `sources.All`.
- **Discovery (listing)**: a company's page (`herp.careers/v1/<board>`) is server-rendered HTML
  linking either directly to job postings (`/v1/<board>/<jobID>`) or to "requisition groups"
  (`/v1/<board>/requisition-groups/<uuid>`) — themselves one more listing page of the same
  direct-job-link shape, one level deep, no further nesting or pagination observed. The adapter
  walks both levels using the shared `jobLinks` DOM helper, matching the URL path shape exactly
  (never a substring match, so an unrelated host's link — e.g. a page's own Twitter/Facebook
  share widget, which embeds the job URL only inside its OWN query string — is never
  misidentified).
- **Detail**: each job page's `application/ld+json` `JobPosting` block (`title`, `description`,
  `datePosted`, `jobLocation.address`) is read via the existing `ldJobPosting` helper, the same
  mechanism `breezy`/`teamtailor` already use — no new parsing primitive.
- **Fully board-listing (`fullBoardListing`)**: a failure fetching the company page OR any
  requisition-group page fails the whole `Fetch` call (never a silently truncated partial
  listing) — only a per-JOB detail fetch failure is best-effort (`unreadableDetail`), matching
  every other `fullBoardListing` adapter's contract.
- **Board recognition**: one new `atsboard` entry, `herp.careers` → `herp`, `path` mode with
  `v1` reserved (the board sits at `/v1/<board>/…`, the same reserved-leading-segment shape
  Gusto's `/boards/<board>` already uses).
- **Harvest discovery**: no bespoke `cmd/harvest-boards` prober — `herp` becomes a board-keyed
  registered provider with no entry in `probers`, so it automatically falls back to
  `adapterProber` (runs the real adapter as the probe), the same path several existing
  HTML-scraping adapters already take.

## Capabilities

### Modified Capabilities
- `source-ingest`: add a requirement that `herp` is a registered board-based provider — a
  two-level HTML link-discovery + ld+json-detail adapter over `herp.careers`, yielding the
  normalized job shape.

## Impact

- New file: `internal/ingest/sources/herp.go` (+ `herp_test.go`).
- `sources.All`: one new registration line.
- `internal/ingest/atsboard/board.go`: one new `atsBoards` row (`path` mode) + one
  `reservedSegments` entry (`"v1"`).
- `cmd/harvest-boards`: no new file — automatic `adapterProber` fallback covers discovery.
- No API, schema, or migration changes.
- Downstream: `internal/ingest/contribution` accepts and rewards HERP board contributions once
  merged (see `internal/ingest/atsdetect`'s guard — HERP is not one of the five shapes it
  protects, so this widening needs no separate proposal the way the Keka change did).
