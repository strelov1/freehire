## Why

eRecruiter (Polska) is the single largest ATS gap behind justjoin: 161 distinct
companies apply through eRecruiter forms, and today we can only re-aggregate those
postings via justjoin — a second-class source with weaker dedup, freshness, and no
self-close. eRecruiter exposes a fully keyless public per-company board
(`skk.erecruiter.pl`), so we can ingest those companies directly under their own
identity instead of through the aggregator.

## What Changes

- Add an `erecruiter` source adapter (`internal/sources/erecruiter.go`) that crawls one
  company's public "Strona Kariera" board. The board id is the company's `cfg`
  (32-hex config token). It lists postings via `GetHtml.ashx?cfg=<cfg>&grid=rows&pn=<n>`
  and fetches each posting's title/location/description from its `Offer.aspx` detail page.
- Register `erecruiter` in `sources.All`, add the board file `sources/erecruiter.yml`
  (one entry per company: `company` + `board`=cfg), and wire an ingest cron for it.
- Add a `cmd/harvest-erecruiter` tool that resolves a company's `cfg` from its careers
  page (fetch page → extract `Code.ashx?cfg=<hex>`), live-validates it against the board
  endpoint, and prints ready-to-paste `sources/erecruiter.yml` entries — the discovery
  bridge for the 161 justjoin companies (justjoin exposes per-job `WebID` apply forms,
  which carry no `cfg`).

## Capabilities

### New Capabilities
- `erecruiter-source`: a board-based `erecruiter` adapter that crawls one company's
  keyless eRecruiter career board (list rows + per-offer detail) into the catalogue,
  plus a harvest prober that discovers and validates a company's `cfg` board token.

### Modified Capabilities
<!-- None: source-ingest's registry/board-file contract is unchanged; this adds a new provider under it. -->

## Impact

- New: `internal/sources/erecruiter.go` (+ test), `sources/erecruiter.yml`,
  `cmd/harvest-erecruiter/main.go`.
- Modified: `internal/sources/source.go` (`All` registry line), Dockerfile/cron wiring
  for the new ingest board file (per-provider schedule).
- No schema/migration changes. Postings flow through the existing `pipeline.Runner` →
  `UpsertJob` write path; the stale-job sweep and incremental indexing apply unchanged.
- eRecruiter boards carry all of a company's jobs (incl. non-IT); the existing
  classify/skilltag dictionaries filter/derive facets as for any other source.
