## 1. Rolling request-error window (`internal/platform/observability`)

- [x] 1.1 RED: write `requestwindow_test.go` covering no-traffic (zero rate), a
      mixed-response rate within a window, and exclusion of responses outside
      the window.
- [x] 1.2 GREEN: implement `requestWindow` (per-minute buckets, mutex-guarded)
      with `record`/`errorRate`, plus the package-level `RecordRequest`/
      `ErrorRate` convenience functions over a singleton instance.
- [x] 1.3 RED: write tests pinning that `HTTPMetrics()` and `CountErrors()`
      both feed the window (mirroring the existing Prometheus-counter tests
      in `httpmetrics_test.go`).
- [x] 1.4 GREEN: call `RecordRequest(status)` from both `HTTPMetrics()` and
      `CountErrors()`, alongside the existing counter increments.
- [x] 1.5 Verify: `go test ./internal/platform/observability/...` green.

## 2. Site status derivation (`internal/api/handler`)

- [x] 2.1 RED: add `TestDeriveSiteStatus` table to `status_test.go` covering:
      DB down → down (regardless of error rate); DB up + no traffic signal →
      operational; DB up + too little traffic → operational even at 100%
      error rate; DB up + moderate error rate with enough traffic → degraded;
      DB up + error rate at/above the down threshold → down.
- [x] 2.2 GREEN: implement `deriveSiteStatus(dbUp bool, errorRate float64,
      totalRequests int64) providerStatus` and the named threshold constants
      (`siteDegradedErrorRate`, a down threshold, a minimum-traffic
      threshold) in `status.go`.
- [x] 2.3 Verify: `go test ./internal/api/handler/... -run DeriveSiteStatus`
      green.

## 3. Wire site status into the handler and response

- [x] 3.1 Add a `*pgxpool.Pool` field to `statsHandlers`; update
      `newStatsHandlers` to accept it and the call site in `handler.go` to
      pass `cfg.Pool`.
- [x] 3.2 In `IngestStatus`, ping the pool, read `observability.ErrorRate`
      over a fixed window constant (10 minutes), derive the site status, and
      add a `site` object (`status`, `database`, `error_rate`,
      `window_minutes`) to the JSON response alongside the existing
      `overall`/`providers`/`last_job_added_at` fields.
- [x] 3.3 Extend `status_integration_test.go` (or add a focused test) to
      assert the `site` object is present and shaped correctly against a
      real pool.
- [x] 3.4 Verify: `go vet ./...`, `go test ./...`, and
      `go vet -tags=integration ./...` all pass; run the integration test
      itself if Docker is available.

## 4. Frontend

- [x] 4.1 Add a `SiteHealth` type to `web/src/lib/types.ts` and a `site:
      SiteHealth` field on `IngestStatus`, reusing the existing
      `HealthStatus` union.
- [x] 4.2 Update the `ingestStatus()` doc comment in `web/src/lib/api.ts` to
      reflect the broadened response (no signature change).
- [x] 4.3 Add a "Site status" block to `StatusBoard.svelte`, rendered above
      the existing ingest-fleet banner/list, reusing `STATUS_META` for the
      pill/dot styling; check whether a short human label per status
      (mirroring `OVERALL_HEADLINE`) reads better than the raw status word.
- [x] 4.4 Manually verified against a real local Postgres + `cmd/server` +
      the SvelteKit dev server: `/status` renders both sections correctly
      for the "all operational" case, AND for a real database outage
      (stopped the container mid-request). The outage check caught a real
      bug — see the "Database check ordering" note added to design.md — now
      fixed and re-verified (200 with `site.status: "down"`, recovers to
      "operational" once the database comes back).

## 5. Finish

- [x] 5.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`,
      `go vet -tags=integration ./...`.
- [x] 5.2 Run `simplify` pass over the diff (extracted `statusResponse` to
      remove the duplicate JSON envelope between the healthy path and the
      database-down short-circuit).
- [x] 5.3 Code review pass on the full diff. Fixed: `overall` no longer
      reads `"down"` when the database check fails (now the same
      "no data" `"operational"` value an empty rollup already reports —
      `site.status` alone carries the database's own down verdict);
      `requestWindow` now prunes on every write, not only on read, bounding
      memory even if `/api/v1/status` goes unpolled; `HTTPMetrics`/
      `CountErrors`' duplicated 3-line recording was extracted into one
      `recordResponse` helper. Re-verified manually (real DB outage via a
      stopped container) and re-ran the full test suite — see design.md's
      "Code review outcome" section for the fixes and the points
      deliberately left as already-reasoned trade-offs.
- [x] 5.4 Finish the branch, archive and sync the OpenSpec change. Merged
      as #2559, deployed to prod via `release.sh` (blue/green flip to
      blue), and verified live: `GET /api/v1/status` carries the `site`
      object, `/status` renders the new section, and no 5xx responses
      followed the flip.
