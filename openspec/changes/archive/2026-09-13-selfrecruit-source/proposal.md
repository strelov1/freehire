## Why

A contributor submitted a live selfrecruit.ge posting (`board_submissions` id 8,
`dressup.selfrecruit.ge/a7cdcc00-1c9c-464c-8960-945af0c0e0a4`) that the recognizer
correctly declined to turn into a board, because no `selfrecruit` adapter exists to
crawl it. selfrecruit.ge (branded "Self.hr" for the Georgian market) is a real
multi-tenant ATS — each client gets its own `<tenant>.selfrecruit.ge` subdomain — so
this is a genuine adapter gap, not a vanity domain.

## What Changes

- Add a `selfrecruit` source adapter that crawls one tenant's public job listing and
  detail pages. The board id is the tenant subdomain (e.g. `dressup`).
- The platform exposes no JSON API: the listing is paged at
  `https://<board>.selfrecruit.ge/vacancies/<offset>` (steps of 10) and server-renders
  bare-root `<a href="/<uuid>">` links to detail pages, and each detail page is raw HTML
  with no schema.org/ld+json markup — title and description come from DOM extraction (via
  the shared `html.go` helpers already used by `successfactors`), not JSON decoding.
- Register `selfrecruit/dressup` in the board catalog via `cmd/add-board` once this
  ships, closing `board_submissions` id 8.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `source-ingest`: add a requirement that `selfrecruit` is a registered board-based
  provider — an HTML listing + DOM-extracted-detail adapter over `<board>.selfrecruit.ge`,
  yielding the normalized job shape with a UUID-derived `external_id`.

## Impact

- New file `internal/ingest/sources/selfrecruit.go` (+ test), registered in the source
  registry alongside the other adapters.
- `internal/ingest/atsboard`: a new host rule so a future `*.selfrecruit.ge` link
  resolves live instead of landing back in `board_submissions`.
- Frontend source-facet enum (`contracts.ts`) regenerated to include `selfrecruit`, same
  as the `herp`/`hrmos` adapters.
- No database migration; the board itself is added by hand via `cmd/add-board` after
  merge, per the existing curator workflow.
