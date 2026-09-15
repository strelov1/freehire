## Why

Recrutei (`jobs.recrutei.com.br/<board>`) is a real multi-tenant Brazilian ATS found while
draining `board_submissions` (id 115, `jobs.recrutei.com.br/digisystem/vacancy/157190-...`).
An earlier drain pass left it as an unbuilt lead because static probing of the listing page
found no embedded job array — the actual listing endpoint (a POST, not a GET) was only found
by capturing real browser network traffic (CDP), since several plausible REST-path guesses
turned out to be the platform's own SPA catch-all `index.html` rather than JSON.

## What Changes

- Add a `recrutei` source adapter (`internal/ingest/sources/recrutei.go`).
- Listing: one `POST https://api.recrutei.com.br/api/v2/vacancies/per-departments/<board>`
  with body `{"search":""}` returns every open posting for the tenant in one response,
  grouped by department, with `data.total` equal to the sum of every department's item
  count (verified live: 198 == 122+3+5+68 for `digisystem`) — confirmed to need no
  pagination.
- Each listing item already carries id/title/company name/location/regime (the Brazilian
  employment-contract type: CLT/PJ/etc.)/the detail page's public URL — everything except
  the posting's own description and post date, which come from a per-posting detail fetch.
- Detail: each item's `public_link` page embeds a standard schema.org
  `application/ld+json` `JobPosting` block, reused via the SHARED `ldJobPosting` decoder
  (`internal/ingest/sources/jsonld.go`, the same one `geekhunter` already uses) rather than
  parsing it again from scratch. Only `description` and `datePosted` are read from it — the
  block's own `jobLocation`/`employmentType` fields are unreliable (see design.md) and the
  listing's own fields are used instead.
- Register `recrutei` in `sources.All` and add an `internal/ingest/atsboard` recognizer
  entry (`jobs.recrutei.com.br`, path mode: board = the first path segment).
- Register `recrutei/digisystem` via `cmd/add-board` once merged and deployed, closing
  `board_submissions` id 115.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `source-ingest`: add a requirement that `recrutei` is a registered board-based provider —
  a listing-enumerates/detail-hydrates adapter over `jobs.recrutei.com.br/<board>`, yielding
  the normalized job shape including a Brazilian labor-regime-derived employment type from
  the listing and a rich HTML description from the detail page's ld+json block.

## Impact

- New files `internal/ingest/sources/recrutei.go` (+ test).
- `internal/ingest/sources/registry.go`: one new registration line.
- `internal/ingest/atsboard/board.go`: one new `atsBoards` row (`path` mode) + a
  `TestRecognize` case.
- Frontend source-facet enum (`contracts.ts`) regenerated to include `recrutei`.
- No changes to `internal/ingest/sources/jsonld.go` — this PR only consumes the existing
  `ldJobPosting` decoder.
