## Why

`app.tecla.io` is a remote-only marketplace of LatAm engineering vacancies behind a
clean, paginated public JSON API. It is a worthwhile catalogue source, but it does not
fit any existing adapter shape: unlike every current source it is an **aggregator** whose
postings each carry their **own** employer, so the company cannot come from board-file
config the way it does for a single-company (`uber`, `amazon`) or per-tenant (`greenhouse`)
board.

## What Changes

- Add a `tecla` source adapter (`internal/sources/tecla.go`) implementing `Source` and the
  `boardless` marker, paginating `https://api.tecla.io/api/jobs/getPublicJobs/?page=N` over
  `data.pagination.countPages` (bounded by a defensive max-page cap).
- Map each posting to `sources.Job`: `ExternalID=id`, `URL=https://app.tecla.io/job?id=<id>`,
  `Title=name`, **`Company=` the posting's own `company.name`** (the new aggregator behavior),
  `Description=` the public (sanitized) text, `Remote=true` + `WorkMode="remote"` (the platform
  is remote-only), `PostedAt=createdAt` parsed with a no-timezone layout.
- Register `NewTecla(c)` in `sources.All` and add `sources/tecla.yml` (one entry: `company: Tecla`,
  `provider: tecla`; boardless, so no `board`).
- Out of scope (intentional): salary (no field on `Job`), skills/seniority parsing (the ingest
  dictionaries own that), and the auth-gated full-description fetch — the public API truncates
  the description and we take it as-is. The daily cron lives in the separate `freehire-ops` repo.

## Capabilities

### New Capabilities
<!-- none — this extends the existing source-ingest capability -->

### Modified Capabilities
- `source-ingest`: adds the requirement that an **aggregator** provider derives each job's company
  from the posting itself rather than from the configured board entry, and registers `tecla` as a
  boardless provider.

## Impact

- New: `internal/sources/tecla.go`, `internal/sources/tecla_test.go`, `sources/tecla.yml`.
- Modified: `internal/sources/source.go` (one line in `All`).
- No schema, migration, or API change. New jobs flow through the existing `UpsertJob`/enrichment
  path unchanged; reaching the catalogue is a `freehire-ops` cron addition (out of this repo).
