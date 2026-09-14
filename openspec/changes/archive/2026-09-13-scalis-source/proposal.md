## Why

Scalis (`<board>.scalis.ai`) is a real multi-tenant ATS found while draining
`board_submissions` (id 29, `boldbusiness.scalis.ai/job/<uuid>`). It is a Next.js App
Router site with no `__NEXT_DATA__`/ld+json, but this codebase already has an RSC-flight
decoder (`internal/ingest/sources/nextflight.go`, used by `deel`/`vouch`/`topco`/`micro1`/
`alignerr`/`remotedotcom`) built for exactly this shape. Live investigation confirms
Scalis's listing page inlines a complete, richly-structured job object per posting — title,
company, locations, employment/workplace enums, skills, salary, and a full HTML
description reachable as a flight text-row reference — so this is a clean adapter to add on
top of the existing primitive, not a new decoding problem.

## What Changes

- Add a `scalis` source adapter (`internal/ingest/sources/scalis.go`) using the existing
  `fetchFlight`/`bracketSlice`/`nextFlightTextRows` primitives, following the `deel`
  adapter's shape closely.
- Unlike `deel`'s single-page catalogue, Scalis's listing is paginated
  (`https://<board>.scalis.ai/jobs?page=N&limit=10&sortBy=SORT_BEST_MATCH`, confirmed live:
  10 results per page, a `count`/`paginationCount` total, an empty `results` array past the
  last page — no redirect trap). The adapter pages to exhaustion.
- Register `scalis` in `sources.All` and add an `internal/ingest/atsboard` recognizer entry
  (subdomain mode) so a future `*.scalis.ai` link resolves live.
- Register `scalis/boldbusiness` via `cmd/add-board` once merged and deployed, closing
  `board_submissions` id 29.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `source-ingest`: add a requirement that `scalis` is a registered board-based provider —
  a paginated RSC-flight listing adapter over `<board>.scalis.ai`, yielding the normalized
  job shape including structured employment type, work mode, skills, and salary.

## Impact

- New files `internal/ingest/sources/scalis.go` (+ test).
- `internal/ingest/sources/registry.go`: one new registration line.
- `internal/ingest/atsboard/board.go`: one new `atsBoards` row (`subdomain` mode) + a
  `TestRecognize` case.
- Frontend source-facet enum (`contracts.ts`) regenerated to include `scalis`.
- No `internal/ingest/sources/nextflight.go` changes — this PR only consumes the existing
  decoder, mirroring how `deel`/`topco`/`micro1` already do.
