## 1. Schema & DB access

- [x] 1.1 Add `migrations/00NN_job_daily_stats.sql`: create `job_daily_stats(day date PRIMARY KEY, added int NOT NULL DEFAULT 0, removed int NOT NULL DEFAULT 0, computed_at timestamptz NOT NULL DEFAULT now())`. No new `jobs` index — the recompute is a full-table batch aggregate (seq scan); note the bounded-window / partial-index seam in a comment (per design D4/risks).
- [x] 1.2 Add `internal/db/queries/stats.sql`: the atomic-rebuild pair (`DeleteAllJobDailyStats :exec` + `RebuildJobDailyStats :exec` — INSERT … SELECT of the FULL OUTER JOIN of per-day `GROUP BY date(created_at)` and `GROUP BY date(closed_at) WHERE closed_at IS NOT NULL`, UTC dates), plus one dense range read (`ListJobActivity(unit, from, to) :many`) using `generate_series` LEFT JOIN the rollup with a parameterized `date_trunc(unit, …)` grouping (`unit` is the caller-whitelisted granularity). No live-slice query — today's freshness comes from intra-day cron (design D4).
- [x] 1.3 Run `make sqlc` and commit the regenerated `internal/db` code; `go build ./...` green.

## 2. Rollup worker

- [x] 2.1 Write `cmd/rollup-stats/main.go` via `worker.Main`/`worker.Bootstrap`: open a tx on the pool, run `DeleteAllJobDailyStats` then `RebuildJobDailyStats` (atomic rebuild), commit, log a summary (row/day count); run-once-and-exit, non-zero exit on failure. `go vet ./...` green. (Correctness is covered by the 5.1 integration test — the worker has no pure Go logic to unit-test; per design D6 there is no `internal/statsrollup` package.)

## 3. Public read endpoint

- [x] 3.1 RED: unit-test the pure read helper `parseActivityQuery(granularity, from, to)` — asserts default `granularity=day`, the day/week/month whitelist → trunc unit + default range, explicit `from`/`to` override, and an unknown granularity returns an error (→ 400).
- [x] 3.2 GREEN: implement the helper + `internal/handler/stats.go` (`JobsActivity`): parse via the helper (invalid → `fiber.NewError(400)`), pick the matching sqlc query, map rows to the `{"data":[{period,added,removed}],"meta":{granularity,from,to}}` envelope. Register the route in `handler.Register` (public, before any `:slug` routes; no middleware).
- [x] 3.3 Add a handler test asserting the envelope shape and that no auth is required (`200` unauthenticated); re-run tests green.

## 4. Frontend `/trends` page

- [x] 4.1 Add the API client call in `web/src/lib/api.ts` (typed `fetchJobsActivity(granularity, from?, to?)`).
- [x] 4.2 Build `web/src/lib/components/ActivityBars.svelte`: hand-rolled viewBox SVG grouped bar chart — green added / red removed `<rect>` per period, baseline axis, hover labels (follow the `PipelineFunnel`/`RateDonut` pattern). Add a small unit test for its pure layout/scaling helper.
- [x] 4.3 Add the route `web/src/routes/trends/+page.svelte` (+ `+page.server.ts` for SSR initial load): render `ActivityBars`, a day/week/month granularity toggle that re-fetches, `Seo` tags, and empty/loading states.
- [x] 4.4 Add a nav/footer link to `/trends`.
- [x] 4.5 Verify the page: `svelte-check` + local visual check (dev server / throwaway screenshot) that both bars render and the toggle re-aggregates.

## 5. Integration & wiring

- [x] 5.1 Integration test (`//go:build integration`) exercising the recompute against a real Postgres (testcontainers): seed jobs with created/closed/reopened states, run the recompute, assert `job_daily_stats` rows, then read via the endpoint and assert day vs. week aggregation.
- [x] 5.2 Update ops docs / CLAUDE.md worker list + Commands section to include `cmd/rollup-stats` (intra-day cron, ~every 3h) and the new endpoint/page.
- [x] 5.3 Full `go build ./...`, `go vet ./...`, `go test ./...`, and web check/test/lint green.
