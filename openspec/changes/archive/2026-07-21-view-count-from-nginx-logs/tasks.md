## 1. Prerequisite: confirm the live log

- [ ] 1.1 On host2, confirm `access_log` is enabled, capture its path, the active
  `log_format`, a sample `GET /jobs/<slug>` line, and the logrotate cadence +
  retained history depth (`sudo nginx -T | grep -E 'access_log|log_format'`,
  `grep 'GET /jobs/' /var/log/nginx/access.log | tail`, `cat /etc/logrotate.d/nginx`).
  Record the confirmed format in the design's Open Questions before coding the parser.

## 2. Schema

- [x] 2.1 Add a migration creating `job_daily_views(day date, job_id bigint,
  uniques int, PRIMARY KEY (day, job_id))` and `processed_view_logs(day date
  PRIMARY KEY, processed_at timestamptz DEFAULT now())`. (`jobs.view_count`
  already exists.)
- [x] 2.2 Regenerate sqlc after adding queries in later tasks (`make sqlc`).

## 3. Log parsing (internal/viewlog)

- [x] 3.1 RED/GREEN: parser `ParseLine(line) (rec, ok)` extracting client IP,
  User-Agent, method, request path, and status from the confirmed access-log
  format; unparseable lines return `ok=false`. Unit tests cover a valid page line,
  a valid API line, and a malformed line.
- [x] 3.2 RED/GREEN: signal classifier turning a parsed record into
  `{slug, kind: page|api}` or "ignore" — matches `GET /jobs/<slug>` and
  `GET /api/v1/jobs/<slug>` with 2xx only; rejects other paths, methods, statuses.
  Unit tests for each accept/reject case.
- [x] 3.3 RED/GREEN: known-bot User-Agent check applied to the `page` signal only;
  `api` signal bypasses it. Unit tests for a bot UA on both paths.

## 4. Daily aggregation (internal/viewlog)

- [x] 4.1 RED/GREEN: `Aggregate(reader)` that takes each view's day from the line
  timestamp (UTC) and dedups by `(hash(IP+UA), slug, day)`, returning per-`(day,
  slug)` unique counts. Unit tests cover repeat-visitor collapse, distinct-visitor
  separation, same-visitor-two-days, and mixed page/api signals for the same slug.
- [x] 4.2 RED/GREEN: list rotated files under the log dir (skip the live
  `access.log`), gzip-aware open, and expose each file's identity `(device, inode)`.
  Unit tests over temp plain + `.gz` fixtures.

## 5. DB queries

- [x] 5.1 Add `ResolveSlugsToJobIDs` (`WHERE public_slug = ANY($1)`) query; unknown
  slugs simply absent from the result.
- [x] 5.2 Add the batched apply query: upsert `job_daily_views` rows for a day and
  `UPDATE jobs SET view_count = view_count + delta FROM (VALUES ...)` in one call.
- [x] 5.3 Add cursor queries: `IsFileProcessed(device, inode)` and
  `MarkFileProcessed(device, inode, filename)`.

## 6. Worker (cmd/rollup-views)

- [x] 6.1 RED/GREEN: daily mode — for each rotated file not in `processed_view_logs`,
  aggregate, resolve slugs, apply the additive batch, then mark the file processed
  (mark only after the apply commits). Integration test (build-tagged, testcontainers)
  asserting counter + rollup + marker state.
- [x] 6.2 RED/GREEN: `--backfill` mode — walk all retained rotated files, processing
  each unmarked file via the same path; already-marked files skipped. Integration
  test over multi-file fixtures asserting idempotency on re-run.
- [x] 6.3 RED/GREEN: no-op when the access-log path is missing — clean exit, no
  writes. Unit/integration test with a nonexistent path.
- [x] 6.4 Wire flock + run-once lifecycle consistent with other rollup workers.

## 7. Retire the read-path bump

- [x] 7.1 RED/GREEN: remove the `bump` CTE from `RecordJobView` so `POST /view`
  records only `user_jobs.viewed_at` and never changes `view_count`. Update/adjust
  the existing query test to assert `view_count` is unchanged by a view beacon.

## 8. /open engagement figure

- [x] 8.1 RED/GREEN: change `GetEngagementStats` so `viewed` is `SUM(jobs.view_count)`
  while `saved`/`applied` stay `user_jobs`-based. Update the engagement integration
  test to assert `viewed` reflects `view_count`, and empty DB still returns zeros.

## 9. Docs & ops handoff

- [x] 9.1 Add `internal/viewlog/AGENTS.md` and a `cmd/rollup-views` line to the
  root AGENTS.md command list + module map.
- [x] 9.2 Document the required `freehire-ops` changes (dedicated JSON `log_format`
  + scoped `access_log` for `/jobs/` and `/api/v1/jobs/`, and a systemd timer with
  its own flock) in the change/design so ops can land them; note dev is a no-op.

## 10. Verification

- [x] 10.1 `go build ./... && go vet ./... && go test ./...`; run the build-tagged
  integration tests; confirm the read path issues no view-count write (trace/log).
