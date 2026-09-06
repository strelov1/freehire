## Why

`werecruit.io` is the last platform in issue #2080's shortlist — the four platforms whose board id
is derivable from a public URL, ranked by how often an aggregator's apply links carried them.
`dayforcehcm` and `jobappnetwork` are done; `gr8people`/`workgr8` is done. This closes out the
issue's ranked list.

## What Changes

- New `internal/ingest/sources` adapter, provider key `werecruit`. A board is
  `<locale>/<tenant>` exactly as the platform's own public URLs carry it
  (`careers.werecruit.io/<locale>/<tenant>/...`) — verified live that locale is load-bearing, not
  cosmetic: a tenant configured for only one locale answers every OTHER locale with an empty
  list (confirmed: `/fr/idiap` answers zero postings, `/en/idiap` answers four).
- Fetch is a single keyless `GET` of the tenant's listing page
  (`careers.werecruit.io/<locale>/<tenant>`), which embeds the tenant's WHOLE open-postings list
  server-side as a `window.allOffers = [...]` JSON array — no pagination exists or is needed.
  Each posting's description is not in the listing, so it comes from a bounded-concurrency
  per-posting detail fetch (a server-rendered `description` block on the posting's own page,
  already linked by the listing's own `Url` field) — the same shape
  `internal/ingest/sources/factorial.go` uses for typically-small boards, not a `HydratingSource`.
- New row in `internal/ingest/atsboard`'s `atsBoards` table (`modePathLocale`, since the pattern —
  board = tenant segment after an optional/required leading locale — already exists for Rippling)
  so a person-pasted or harvested `careers.werecruit.io/<locale>/<tenant>/offers/<slug>` link
  resolves to `(werecruit, "<locale>/<tenant>")`.
- One line in `sources.All` (`registry.go`).
- `internal/ingest/sources/AGENTS.md` gains a "werecruit traps" section.
- `make gen-contracts` run and its diff committed (required whenever a new provider key is
  added — the `jobappnetwork` PR's CI caught this the hard way).

Not in scope: none — this is the last platform in issue #2080's shortlist.

## Capabilities

### New Capabilities
- `werecruit-source`: crawling one werecruit employer board (a locale-scoped tenant career site)
  through its embedded listing data, hydrating each posting's description, and recognizing the
  platform's public URL shape for board onboarding.

### Modified Capabilities
(none)

## Impact

- `internal/ingest/sources/werecruit.go` (+ `_test.go`): new adapter.
- `internal/ingest/sources/registry.go`: one new `registry["werecruit"] = ...` (or `reg(...)`)
  line.
- `internal/ingest/atsboard/board.go`: one new `atsBoards` row.
- `internal/ingest/sources/AGENTS.md`: one new "traps" section.
- `web/src/lib/generated/contracts.ts`: regenerated.
- No schema change, no new env var, no worker change.
