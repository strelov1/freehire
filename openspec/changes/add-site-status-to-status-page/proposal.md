## Why

The public `/status` page (`openspec/specs/ingest-status-page`) only reports the
health of the ingest fleet — how many ATS boards are being crawled successfully.
It says nothing about whether the site/API itself is up, so a visitor cannot tell
"is freehire.me itself working" from that page, even though that is the more
common thing a status page is expected to answer. `GET /health` (DB ping only)
and the Prometheus counters in `internal/platform/observability` already carry
enough signal to answer this without a new external dependency.

## What Changes

- Add an in-process rolling request window (`internal/platform/observability`)
  that tracks total/5xx response counts per minute, fed by the existing
  `HTTPMetrics()`/`CountErrors()` middleware. This answers "what fraction of
  recent responses failed" without querying Prometheus or any other external
  system — the same process serving the status request is the one being asked
  about, so a live answer is definitionally accurate for the process handling
  it.
- Add a pure `deriveSiteStatus` classifier (`internal/api/handler`) folding a
  live DB ping and the rolling error rate into one `operational` / `degraded`
  / `down` verdict, mirroring the existing `deriveStatus`/`fleetStatus`
  pattern already used for the ingest fleet.
- Extend the existing `GET /api/v1/status` response with a new `site` object:
  `{ status, database, error_rate, window_minutes }`.
- Extend the `/status` page with a new "Site status" section, rendered above
  the existing ingest-fleet provider list, using the same status-pill styling.
- No new database table, no new cron worker, no new external dependency, no
  change to the existing 60s page-level cache.

## Capabilities

### New Capabilities
- `site-health-status`: in-process request-window tracking, DB-ping check, and
  the derived operational/degraded/down verdict for the site/API itself
  (independent of the ingest fleet's own status).

### Modified Capabilities
- `ingest-status-page`: `GET /api/v1/status` response gains a `site` object;
  the `/status` page gains a "Site status" section above the existing
  provider list.

## Impact

- `internal/platform/observability/requestwindow.go` (new), `httpmetrics.go`
  (wires `RecordRequest` into the existing middleware).
- `internal/api/handler/status.go` (site classification + response field),
  `internal/api/handler/stats.go` / `handler.go` (thread `*pgxpool.Pool` into
  `statsHandlers` for the DB-ping check).
- `web/src/lib/types.ts`, `web/src/lib/components/StatusBoard.svelte`, and
  (doc-comment only) `web/src/lib/api.ts`.
- No migration, no new worker, no change to `freehire-ops`.
