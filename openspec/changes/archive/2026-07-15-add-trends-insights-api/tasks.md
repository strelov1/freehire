# Tasks

Each implementation task runs the spec-driven-tdd micro-cycle:
RED → GREEN → REFACTOR → simplify → re-test → review → mark `[x]`.
Tasks are ordered so each is verifiable on its own.

## 1. Rollup schema & recompute (data layer)

- [x] 1.1 Add migration creating `insights_role_stats`, `insights_skill_stats`,
      `insights_salary_stats`, and `insights_velocity_daily` tables with primary
      keys and any read indexes; document the manual prod-apply order in the file
      header (per the migrations gotcha).
- [x] 1.2 Write recompute SQL queries (delete-all + rebuild) for each rollup in
      `internal/db/queries/insights.sql`, using open-as-of-date derived from
      `created_at`/`closed_at`; salary rollup computes per-(currency,period)
      `percentile_cont` p25/p50/p75 and emits only bands at/above the min sample
      size. Regenerate sqlc (`make sqlc`).
- [x] 1.3 Write the read queries for each endpoint (role ranking by open_count or
      growth; skill ranking; faceted velocity dense series; salary bands) in the
      same file; regenerate sqlc. Verify generated code compiles (`go build ./...`).

## 2. Rollup worker

- [x] 2.1 Extend `cmd/rollup-stats` to recompute the four `insights_*` tables in
      the same atomic transaction as `job_daily_stats` (delete + rebuild swap),
      logging per-table row counts; keep re-runs idempotent.
- [x] 2.2 Add/extend an integration test (testcontainers, `//go:build integration`)
      that seeds jobs and asserts the rollups match a hand-computed expectation,
      including the salary min-sample-size suppression and open-as-of-date growth.

## 3. Handler: validation & envelope

- [x] 3.1 Create `internal/handler/insights.go` with whitelist-validating parsers
      for shared params (geography enum, `sort`, bounded `limit`, and the
      velocity window/granularity reusing the `stats.go` pattern with an injected
      `now`). Unit-test the parsers (defaults, rejections, bounds).
- [x] 3.2 Implement `GET /api/v1/insights/roles` handler mapping rollup rows to the
      `{"data","meta"}` envelope; unit-test envelope shape, geography scoping,
      sort=growth, and 400 on invalid params.
- [x] 3.3 Implement `GET /api/v1/insights/skills` handler; unit-test ranking,
      category/geo scoping, and envelope.
- [x] 3.4 Implement `GET /api/v1/insights/velocity` handler producing the dense
      series with optional single-facet scoping; unit-test dense zeros, facet
      scoping, and range-too-large 400.
- [x] 3.5 Implement `GET /api/v1/insights/salary` handler; unit-test per-currency
      separation, band fields, and small-sample suppression.

## 4. Routing & wiring

- [x] 4.1 Register the four routes as public (no middleware) under `/api/v1/insights`
      in `internal/handler/handler.go`, grouped next to `/stats`.
- [x] 4.2 Add a handler DB integration test (`//go:build integration`) hitting each
      route end-to-end against a populated rollup and asserting aggregate-only
      payloads (no slug/id/title/description leak).

## 5. Docs & ops

- [x] 5.1 Document the four endpoints (params, response shape) in `docs/API.md` and
      add them to `web/static/openapi.yaml`.
- [x] 5.2 Note the new rollup cron requirement and manual prod-migration step for
      the ops repo (record in the change; no code in this repo's cron).

## 6. Verify & finish

- [x] 6.1 `go build ./... && go vet ./... && go test ./...` green; run the
      integration tests where Docker is available.
- [x] 6.2 verification-before-completion: run the server locally, populate rollups
      via the worker against a seeded DB, and curl each endpoint to confirm real
      aggregate output.
