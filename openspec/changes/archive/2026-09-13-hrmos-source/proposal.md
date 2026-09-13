## Why

HRMOS (`hrmos.co`, by Bizreach) is a Japanese multi-tenant ATS with ~961 companies in an
external inventory (kalil0321/ats-scrapers' `ats-companies/hrmos.csv`) freehire currently
cannot crawl at all — no adapter, no board recognition, no harvest path. Live investigation
(the same discipline just applied to the `herp-source` change) confirms it fits the project's
existing adapter shape cleanly and is, if anything, simpler than HERP: the listing is a
standard `?page=N` paginated HTML page (the codebase's existing `crawlPagedLinks` helper
applies directly, no custom multi-level expansion needed), and every job detail page carries a
standard schema.org `JobPosting` `application/ld+json` block — richer than HERP's, including a
schema.org-standard `employmentType` enum that maps cleanly onto freehire's own controlled
vocabulary.

## What Changes

- Add a `hrmos` source adapter (`internal/ingest/sources/hrmos.go`) speaking the existing
  `Source` interface, registered in `sources.All`.
- **Discovery (listing)**: `hrmos.co/pages/<board>/jobs`, paged via `?page=N`, walked with the
  existing shared `crawlAllPagedLinks` helper — the fail-on-any-gap variant, since a silently
  truncated later page would otherwise undercount the board while still returning success. A
  job link SHALL be matched by resolved host and an exact two-segment path shape
  (`jobs/<jobID>` after the board) — never a bare one-segment match, learning directly from
  `herp-source`'s own review finding that a loosely-shaped single-segment match lets platform
  navigation (there, a `/top` landing page) pass as a job.
- **Detail**: each job page's `application/ld+json` `JobPosting` block (`title`, `description`,
  `datePosted`, `jobLocation[].address.*`, `employmentType`) is read via the existing shared
  `ldJobPosting` helper — the same mechanism `herp`/`breezy`/`teamtailor` already use.
  `employmentType` is schema.org's own closed enum (`FULL_TIME`, `PART_TIME`, `CONTRACTOR`,
  `TEMPORARY`, `INTERN`, ...) and maps onto `vocab.EmploymentTypeValues` by case-folding plus a
  small synonym table; an unrecognized value is left empty rather than guessed.
- **`fullBoardListing`**: `crawlAllPagedLinks` fails the whole `Fetch` on ANY page failing, not
  just the first — the plain `crawlPagedLinks` variant would silently return a truncated
  success on a later-page failure, which does not meet the "whole listing or fail outright"
  bar this marker promises. A failure fetching one job's detail marks only that posting
  `Unreadable`.
- **Board recognition**: one new `atsboard` entry, `hrmos.co` → `hrmos`, `path` mode, with
  `reservedSegments["hrmos.co"] = ["pages"]` and the two-segment-after-board shape mirrored in
  the recognizer the same way it is in the adapter.
- **Harvest discovery**: no bespoke `cmd/harvest-boards` prober — falls back to `adapterProber`
  like `herp`.

## Capabilities

### Modified Capabilities
- `source-ingest`: add a requirement that `hrmos` is a registered board-based provider — a
  paginated HTML-listing + ld+json-detail adapter over `hrmos.co`, yielding the normalized job
  shape including a mapped `employment_type`.

## Impact

- New file: `internal/ingest/sources/hrmos.go` (+ `hrmos_test.go`).
- `sources.All`: one new registration line.
- `internal/ingest/atsboard/board.go`: one new `atsBoards` row (`path` mode) + one
  `reservedSegments` entry (`"pages"`).
- `cmd/harvest-boards`: no new file — automatic `adapterProber` fallback covers discovery.
- No API, schema, or migration changes.
