## Why

eArcu is a multi-tenant UK ATS behind the career sites of large employers (Cambridge University Press & Assessment, plus a documented roster including BP, Asda, British Airways, FirstGroup, Flutter, the Football Association, Virgin Active, CVS Group, Cromwell Tools). We currently ingest none of them: eArcu clients run on their own custom domains (`careers.<company>.com`), so their vacancies are invisible to every existing adapter. Each eArcu site publishes a keyless `/jobs/rss` feed with the full posting body inline, so the platform is cleanly crawlable and worth a dedicated adapter (multi-tenant + keyless + structured — the same bar the other ATS adapters clear).

## What Changes

- Add an `earcu` source adapter (`internal/sources/earcu.go`) implementing the `Source` interface: fetch a board's keyless `/jobs/rss` feed, parse each `<item>` into a `Job` (title, posting URL, external id, location, posting date, description body), and fall back to the detail page's schema.org `JobPosting` JSON-LD when a feed item's body is empty.
- Register it in `sources.All` (one line) so `cmd/ingest` and config validation recognise the `earcu` provider.
- Seed `sources/earcu.yml` with verified eArcu career hosts (board = the full careers host, e.g. `careers.cambridge.org`), each validated live before inclusion.
- eArcu is board-based and multi-tenant (not boardless), so it lists in the source facet with no extra wiring — like greenhouse/personio.

## Capabilities

### New Capabilities
- `earcu-source`: crawl a per-company eArcu career site via its keyless `/jobs/rss` feed (board = careers host) and normalize its open vacancies into the job catalogue, with detail-page JSON-LD as a body fallback.

### Modified Capabilities
<!-- None: this is an additive adapter behind the existing Source interface and registry; no existing spec's requirements change. -->

## Impact

- **New code:** `internal/sources/earcu.go` (+ `earcu_test.go`), one registration line in `internal/sources/source.go`, new board file `sources/earcu.yml`.
- **No schema/API changes:** the adapter emits the same `sources.Job` shape every other adapter does; ingest, dedup, enrichment, and search are unchanged.
- **Ops:** a new board file means a new `cmd/ingest sources/earcu.yml` cron entry (added the same way as every other provider file).
- **Reuses existing transport:** `XMLGetter` (RSS) + `HTMLGetter` (JSON-LD detail fallback) and the shared `sanitizeHTML` / `ldJobPosting` helpers — no new HTTP-client capability.
- **Known seam (out of scope):** bulk harvest of the full eArcu customer roster. This change seeds only live-validated hosts; expanding the roster is follow-up discovery work, mirroring how other adapters shipped with a seed and grew later.
