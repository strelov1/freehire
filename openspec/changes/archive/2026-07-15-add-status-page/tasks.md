## 1. Rollup query

- [x] 1.1 Add `ProviderHealthRollup :many` to `internal/db/queries/board_health.sql` (GROUP BY provider: total/healthy/cooled counts via FILTER, max last_run_at/last_success_at, sum ingested over healthy)
- [x] 1.2 Regenerate `internal/db` via sqlc; confirm `go build ./...` passes with the new generated method

## 2. Status derivation policy

- [x] 2.1 RED: table test for the pure status-derivation function — operational (≥90% healthy + fresh), degraded (minority failing), down (≤10% healthy), down-when-stale (healthy counts but no success in 48h), and empty-fleet → operational overall
- [x] 2.2 GREEN: implement `deriveProviderStatus` + overall-status helper + named threshold/freshness constants in `internal/handler/status.go`; tests pass

## 3. Public status endpoint

- [x] 3.1 RED: integration test (build-tag `integration`) — seed `board_health` rows, call `GET /api/v1/status`, assert 200, sanitized DTO shape (overall + providers[]), and absence of `last_error`/board identifiers
- [x] 3.2 GREEN: implement the handler in `internal/handler/status.go` (rollup → derive → sanitized DTO) and wire the public route beside `/health` in `internal/handler/handler.go`; tests pass

## 4. Public status page

- [x] 4.1 Add `web/src/routes/status/` page: SSR-fetch `/api/v1/status`, render overall banner + flat provider list (status pill, total/healthy counts, relative last-run time)
- [x] 4.2 Visual-verify the page (headless Chrome screenshot) in operational and degraded states
