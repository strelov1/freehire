## 1. Fix the shared dedup/write target

- [x] 1.1 In `scripts/ats_boards.py`, replace `existing_slugs()`'s file-based read
      (`SOURCES_DIR / f"{prov}.yml"`) with a live query against the `boards` table
      (`DATABASE_URL`, `psql -t -A`), returning the same `dict[str, set[str]]` shape callers
      already expect. Done as `parse_boards_dump()` (pure, tested) + `_boards_table_dump()`
      (untested IO boundary, like `fetch()`) + `existing_slugs()` (wires them, fails loudly
      via `sys.exit` when `DATABASE_URL` is unset rather than degrading to "nothing tracked").
- [x] 1.2 In `scripts/ats_boards.py`, replace `emit_survivors()`'s `--write` path (append to
      `SOURCES_DIR / f"{prov}.yml"`) with a writer that emits one seed JSON file per provider
      at `scripts/.harvest-seeds/<provider>.json`, in the `{board, company}` shape
      `cmd/harvest-boards`'s `seed.go` already parses. Done via a new `seed_items()` pure
      function (tested) + the write block in `emit_survivors()`; the printed report also now
      prints the exact `go run ./cmd/harvest-boards <provider> <seed> --apply` follow-up
      command for each provider with survivors.
- [x] 1.3 Add `scripts/.harvest-seeds/` to `.gitignore`. Verified with `git check-ignore -v`.
- [x] 1.4 There is no standalone `test_ats_boards.py` — its functions are covered through
      `scripts/test_harvest_boards.py` and `scripts/test_discover_boards.py` (both import
      from `ats_boards`). Added 4 tests to `test_discover_boards.py` (`parse_boards_dump` x3,
      `seed_items` x1) — `existing_slugs()`/`emit_survivors()` themselves stay untested
      integration glue, consistent with this suite's existing convention of not testing
      IO-orchestrating functions (`fetch`, `validate`, `hn_hiring_threads`, etc. are likewise
      untested). Both suites pass: 14/14 (test_discover_boards.py), 10/10
      (test_harvest_boards.py). Additionally smoke-tested end to end against the real local
      dev Postgres (`hire-db-1`) via a psql shim: dedup correctly filtered an already-tracked
      board, live-validated a real untracked Ashby board (openai, 806 jobs) and wrote the
      expected seed JSON.

## 2. Point the aggregator sweep at the current season

- [x] 2.1 In `scripts/harvest_boards.py`, update `AGGREGATORS` to
      `SimplifyJobs/New-Grad-Positions`, `SimplifyJobs/Summer2027-Internships`, and
      `vanshb03/Summer2027-Internships` (drop the stale `Summer2026` URLs; keep the
      `crypto-jobs-fyi` entries as-is — out of scope for this change).
- [x] 2.2 No existing test fixture hardcoded the old URLs. Added
      `test_aggregators_include_current_season_repos` (RED against the stale list, GREEN
      after the update) to guard against a future season silently going stale again. 11/11
      passed.

## 3. Onboard the current backlog

**Deferred to after this change merges** (decided with the user): land the code change
first, review and merge it, then run this group as a separate operational step against
prod. Not part of this PR's diff.

- [ ] 3.1 Run `python3 scripts/harvest_boards.py --write` against `DATABASE_URL` pointed at
      prod (read-only for this step — it only produces seed files, nothing is persisted yet)
      and record the per-provider candidate counts it reports.
- [ ] 3.2 For each provider with a non-empty seed file, run `go run ./cmd/harvest-boards
      <provider> scripts/.harvest-seeds/<provider>.json --apply` against prod.
- [ ] 3.3 Confirm the newly-inserted boards land at `status='pending'` in `boards` (a spot
      query is enough — `cmd/ingest`'s next scheduled run for each affected provider promotes
      them to `active` on first successful crawl, per `internal/ingest/boardcatalog/AGENTS.md`).

## 4. Verify

- [x] 4.1 `gofmt -l .` clean — no output, nothing to format (no Go changed).
- [x] 4.2 `CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./...` — both clean.
- [x] 4.3 `CGO_ENABLED=0 go test ./...` — one pre-existing failure,
      `TestTheStoreProviderAloneKeepsTheWorkerRunning` in `cmd/billing-sync` (a package this
      change never touches). Confirmed unrelated: it fails identically with the local dev DB
      (`hire-db-1`) both running and stopped, and the same failure class (env-dependent tests
      failing on a clean checkout, unrelated to code changes) is exactly what issue #2869's
      author independently reported for three different tests. Every other package passes.
