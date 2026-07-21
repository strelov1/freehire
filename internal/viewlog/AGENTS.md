# internal/viewlog

Parses nginx access-log lines into per-job view counts, **off the request path**.
The read path (`GET /jobs/:slug`, `GET /api/v1/jobs/:slug`) writes no counter; the
`cmd/rollup-views` worker feeds this package a day's log offline and applies the
result to `jobs.view_count` + `job_daily_views`. This keeps the read cheap and
cacheable, and — because the log carries the real client IP/UA (lost at the
SSR→backend boundary) — lets us filter bots and cover anonymous + API uniformly.

## Shape

- `ParseLine(line) (Record, ok)` — one nginx `combined`-format line → `Record`
  (IP, timestamp, method, path, status, UA). Unparseable/bad-request lines → `ok=false`.
- `Classify(Record) (Signal, ok)` — a 2xx GET of exactly `/jobs/<slug>` (`KindPage`)
  or `/api/v1/jobs/<slug>` (`KindAPI`) → the slug; everything else ignored (a slug is
  one path segment, so lists and sub-resources like `/similar`, `/fit` don't count).
- `Aggregate(reader) map[day]map[slug]int` — dedups by `(hash(IP+UA), slug, day)`,
  the day taken from each line's timestamp (UTC); page opens from known bots are
  dropped, API reads are not bot-filtered.
- `RotatedFiles(dir, base)` / `LogFile.Open()` — lists rotated files (skips the live
  `access.log`), exposes each file's `(Device, Inode)` identity (the worker's cursor
  key across numeric-suffix rotation), and opens gzip transparently.

## Conventions

- **Dict/heuristic only, no external calls.** Pure functions over strings + files.
- **Bot list is deliberately small** (`bot.go`): missed bots only inflate a
  transparency number; over-aggressive matching would drop real people.
- **Semantics live here, not in SQL.** The worker's queries are additive plumbing;
  what counts as a view and how it dedups is defined in this package.
