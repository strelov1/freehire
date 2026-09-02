## Why

`jobsearch.createyourowncareer.com` is a live SAP SuccessFactors career site holding 973
postings for 40 different Bertelsmann companies — Arvato Systems, Riverty, RTL, Penguin
Random House, Fremantle, smartclip and more. The successfactors adapter attributes every job
on a board to the one configured company, and the board is ALREADY in
`sources/successfactors.yml` under `company: Arvato` — so the catalogue is currently filing
all 40 employers' postings under Arvato, which is why that slug carries 989 open jobs against
the sitemap's 973.

A link contribution (a Riverty engineering role) is what surfaced the mis-attribution. This
is a correction, not a new board.

The hub concept that solves this already exists in the board-file schema and in three
adapters (huntflow, loxo, cleverstaff); SuccessFactors needs its own way of reading the
employer, because its hub encodes the tenant in the job URL rather than in the posting
payload.

## What Changes

- `sources.CompanyEntry` gains an optional `tenants` map (tenant key → employer display
  name), beside the existing `region` and `hub` fields. It is ignored by adapters that do
  not implement hub resolution.
- The successfactors adapter honours `hub`: it reads the tenant key from the job URL's
  first path segment and resolves the employer through the entry's `tenants` map, falling
  back to the configured company when the key is absent or unmapped.
- `sources/successfactors.yml`'s existing `jobsearch.createyourowncareer.com` entry becomes
  a hub entry: `company: Arvato` becomes `company: Bertelsmann` (the hub's own name and the
  fallback employer) plus `hub: true` and a curated map of the 23 tenant names the platform
  itself states. Arvato keeps its own postings through the `ARVATO` tenant key.
- No breaking change: a board entry without `hub` behaves exactly as before.

## Capabilities

### New Capabilities

None. The hub concept and the board-file schema already live in `source-ingest`.

### Modified Capabilities

- `source-ingest`: the generic hub requirement gains the optional `tenants` map that lets a
  hub entry carry per-tenant employer names, and a new requirement covers how the
  successfactors adapter resolves a hub board's employer from the job URL.

## Impact

- `internal/ingest/sources/source.go` — one optional field on `CompanyEntry`.
- `internal/ingest/sources/successfactors.go` — hub branch in `detail`, plus a `sfTenant`
  URL helper.
- `sources/successfactors.yml` — one existing board entry converted to a hub.
- The catalogue: on the next crawl of this board, roughly 570 of the 989 postings currently
  filed under `arvato` re-attribute to their real employer. The rows are UPDATEd in place
  (`UpsertJob` conflicts on `(source, external_id)`, and the board — which namespaces
  external_id — does not change), so nothing is orphaned. The post-run unseen sweep scopes
  itself by the slugs the run actually PRODUCED (`distinctCompanySlugs` reads `j.Company`),
  not by the board file's company, so per-tenant closes keep working.
- Crawl load: unchanged. The board is already crawled at this size; only the company each
  posting is filed under moves.
- No database, API or web change. Nothing else reads `CompanyEntry.Tenants`.
