## Why

Issue #2017: several providers hold live jobs (`closed_at IS NULL`) whose boards have
failed every crawl attempt for weeks — e.g. `paylocity/3d3c12d8-...` has not had a
successful crawl since 2026-07-29 (41+ days) and is on its 27th consecutive failure,
still cycling through 24h cooldowns. The unseen-job sweep's safety guards
(`shouldSweep`, `boardQualifies`, `sweepableCompanies` — see `job-lifecycle` spec) are
deliberately conservative: they only close a board's stale jobs when THIS run proved
it covered that board. That is correct for a transient failure (a rate limit, a
one-off timeout) but has no upper bound — a board that is permanently gone (retired,
moved, blocking us) never re-qualifies, so its jobs stay open forever with nothing
able to confirm they still exist. Prod measurement (2026-09-08): paylocity 55% of its
live catalogue stale >3 days, jobdanmark 28%, oracle 0.2% — all masked at the
provider level because each provider's OTHER boards keep `Ingested > 0` on every run.

## What Changes

- Board health gains a **chronic** classification: a board with no successful crawl
  for a configurable window (default 30 days) since `last_success_at` — deliberately
  longer than the sweep's cooldown ceiling (~24h) so a board is never mistaken for
  chronic while merely backing off.
- `board_health`'s existing unhealthy-board reporting is extended to call out chronic
  boards distinctly from ordinary cooling/failing ones, so an operator doing the
  regular curation pass (cf. `onboard-contributions`) can decide per board: fix the
  adapter, retire the board from the catalog, or take no action yet.
- A new **safety-net closer** — a separate, deliberately infrequent one-off pass, not
  part of the per-run sweep — closes the still-open jobs of a board that has been
  chronic for a second, longer window (default 60 days total since last success) with
  a distinct close reason, so it is visibly different from an ordinary unseen-sweep
  close and never silently conflated with one. This is the backstop for a chronic
  board nobody has curated yet; a curator acting sooner (e.g. retiring the board)
  pre-empts it.
- No change to the existing per-run sweep's conservative behavior for transient
  failures — `shouldSweep`/`boardQualifies`/`sweepableCompanies` are untouched.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `ingest-board-health`: adds a chronic-board classification and surfaces it distinctly
  in the existing unhealthy-board reporting.
- `job-lifecycle`: adds a safety-net close for jobs whose board has been chronic long
  enough, with its own close-reason mechanism, independent of the per-run unseen sweep.

## Impact

- `internal/platform/db/board_health.sql.go` / `queries/board_health.sql`: a query to
  list boards chronic past the closure window.
- `internal/ingest/pipeline` or a new `cmd/close-chronic-boards`: the safety-net
  closer (one-off worker, following the existing `cmd/backfill-*` pattern — chunked,
  idempotent, dry-run-safe).
- `cmd/ingest/main.go`'s unhealthy-board summary: distinguish chronic boards in the
  logged rollup.
- `jobs.closed_reason`: a new value for this mechanism (see `job-lifecycle`'s "every
  close records the mechanism" requirement).
- No change to `shouldSweep`, `boardQualifies`, or `sweepableCompanies`.
