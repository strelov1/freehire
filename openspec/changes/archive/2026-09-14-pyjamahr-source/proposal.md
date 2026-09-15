## Why

PyjamaHR (`jobs.pyjamahr.com/<board>`) is a real multi-tenant Indian ATS found while draining
`board_submissions` (id 124, `jobs.pyjamahr.com/dodo-payments/backend-engineer-rust-4`). An
earlier drain pass left it as an unbuilt lead because several plausible REST-path guesses
against `app.pyjamahr.com` all turned out to be the platform's own SPA catch-all
`index.html` rather than JSON. The real API — a completely different host,
`api.pyjamahr.com` — was only found by capturing real browser network traffic (CDP).

## What Changes

- Add a `pyjamahr` source adapter (`internal/ingest/sources/pyjamahr.go`).
- Listing: `GET https://api.pyjamahr.com/api/career/jobs/?company_slug=<board>&page=1`, a
  standard DRF-style paginated response (`count`/`next`/`previous`/`results`), paged via its
  own `next` URL until exhausted.
- Each listing item already carries id/slug/title/location/workplace_type — everything
  except the posting's own description and richer structured fields (job type, salary,
  skills, seniority, remote flag), which come from a per-posting detail fetch.
- Detail: `GET https://api.pyjamahr.com/api/career/jobs/<id>/?company_slug=<board>` returns
  the full object — a clean, keyless, self-contained JSON API (no ld+json/RSC-flight
  parsing needed at all, unlike every prior adapter in this initiative).
- Register `pyjamahr` in `sources.All` and add an `internal/ingest/atsboard` recognizer
  entry (`jobs.pyjamahr.com`, path mode: board = the first path segment — the detail page
  is the SAME route with a `?job_uuid=` query param, not a distinct path).
- Register `pyjamahr/dodo-payments` via `cmd/add-board` once merged and deployed, closing
  `board_submissions` id 124.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `source-ingest`: add a requirement that `pyjamahr` is a registered board-based provider —
  a paginated listing-enumerates/detail-hydrates adapter over `jobs.pyjamahr.com/<board>`,
  yielding the normalized job shape including structured job type, workplace type, remote
  flag, salary bounds (only when the platform marks them visible), plain skills, and
  seniority from the detail page's own fields.

## Impact

- New files `internal/ingest/sources/pyjamahr.go` (+ test).
- `internal/ingest/sources/registry.go`: one new registration line.
- `internal/ingest/atsboard/board.go`: one new `atsBoards` row (`path` mode) + a
  `TestRecognize` case.
- Frontend source-facet enum (`contracts.ts`) regenerated to include `pyjamahr`.
