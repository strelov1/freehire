## Why

HumanBit (`jobs.humanbit.ai/<board>`) is a real multi-tenant ATS found while draining
`board_submissions` (id 177, `jobs.humanbit.ai/scrabble-jigsaw/jobs/<uuid>` — the captured
submission itself had since expired to a 404, but the tenant's live board and other
postings confirm the platform live). Like Scalis, it is a Next.js App Router site with no
`__NEXT_DATA__`, decodable via the existing shared RSC-flight primitives
(`internal/ingest/sources/nextflight.go`).

## What Changes

- Add a `humanbit` source adapter (`internal/ingest/sources/humanbit.go`).
- Unlike Scalis, HumanBit's two pages carry complementary, not identical, field sets: the
  listing page embeds a `"jobBoard":"<board>","jobs":[...]` array with id/title/location/
  salary/description-reference/company name — but NOT employment type, skills, remote flag,
  or seniority; the per-posting detail page carries the FULL structured object (all of the
  above, plus `employment_type`, `skills`, `remote`) but not the company display name. The
  listing page is also confirmed live at ~5.6 MB (heavy, likely dominated by framework
  chunks) against a ~59 KB detail page, so the adapter fetches the listing ONCE per crawl
  (for enumeration + company name + the shared description text rows) and fans out one
  detail fetch per posting for the richer structured fields — the same shape
  `hiringthing`/`topco` already use for "cheap enumeration, per-posting detail."
- Register `humanbit` in `sources.All` and add an `internal/ingest/atsboard` recognizer
  entry (path mode: board = the first path segment on `jobs.humanbit.ai`).
- Register `humanbit/scrabble-jigsaw` via `cmd/add-board` once merged and deployed, closing
  `board_submissions` id 177.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `source-ingest`: add a requirement that `humanbit` is a registered board-based provider —
  a listing-enumerates/detail-hydrates RSC-flight adapter over `jobs.humanbit.ai/<board>`,
  yielding the normalized job shape including structured employment type, remote flag, and
  skills from the detail page.

## Impact

- New files `internal/ingest/sources/humanbit.go` (+ test).
- `internal/ingest/sources/registry.go`: one new registration line.
- `internal/ingest/atsboard/board.go`: one new `atsBoards` row (`path` mode) + a
  `TestRecognize` case.
- Frontend source-facet enum (`contracts.ts`) regenerated to include `humanbit`.
- No `internal/ingest/sources/nextflight.go` changes — this PR only consumes the existing
  decoder.
