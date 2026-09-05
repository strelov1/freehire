# internal/application/viewlog

Parses nginx access-log lines into per-job view counts, **off the request path**.
The read path (`GET /jobs/:slug`, `GET /api/v1/jobs/:slug`) writes no counter; the
`cmd/rollup-views` worker feeds this package a day's log offline and applies the
result to `jobs.view_count` + `job_daily_views`. This keeps the read cheap and
cacheable, and — because the log carries the real client IP/UA (lost at the
SSR→backend boundary) — lets us filter bots and cover anonymous + API uniformly.

## Shape

- `ParseLine(line) (Record, ok)` — one nginx `combined`-format line → `Record`
  (IP, timestamp, method, path, status, UA, purpose). Unparseable/bad-request lines
  → `ok=false`. The trailing `"$http_sec_purpose"` field is **optional** in the
  pattern, so lines from before the nginx change parse unchanged; nginx's `-`
  placeholder normalizes to empty.
- `Classify(Record) (Signal, ok)` — a 2xx GET of `/jobs/<slug>` or its SvelteKit SPA
  data request `/jobs/<slug>/__data.json` (`KindPage`), or `/api/v1/jobs/<slug>`
  (`KindAPI`) → the slug; everything else ignored (a slug is one path segment, so
  lists and sub-resources like `/similar`, `/fit` don't count).
- **A non-empty `Sec-Purpose` is never a view.** The app sets
  `data-sveltekit-preload-data="hover"`, so moving the pointer across a listing
  fetches `/jobs/<slug>/__data.json` for every card it passes — each of which
  counted, inflating `view_count` for jobs nobody opened. Any non-empty value is
  rejected rather than matching known ones: the header exists to say "not a real
  navigation", and an unrecognized value still says it. This is also what makes
  Speculation Rules safe to add later — a prerender carries the same header.

**Deploy order matters:** the parser must ship BEFORE the nginx `log_format`
change. It tolerates both formats, so parser-first is a no-op until nginx catches
up; nginx-first would feed lines the old pattern cannot match, and a day that
parses as nothing silently counts nothing.
- `Aggregate(reader) map[day]map[slug]Counts` — dedups by the raw `(IP, UA, slug, day)`
  tuple (NUL-joined, no hashing), the day taken from each line's timestamp (UTC);
  page opens from known bots are dropped, API reads are not bot-filtered.
  `Counts` holds **two** independently-deduplicated visitor counts over that same
  key: `Total` (either signal) and `Page` (page opens only). They are two counts,
  not a count and a breakdown — a visitor who opens the page *and* reads the API on
  the same day is one visitor in each, so they never sum with an API figure, and the
  only relation between them is `Page <= Total`. Putting the signal kind into the
  shared dedup key instead would count that visitor twice in `Total`, and `Total` is
  `job_daily_views.uniques`, which `GET /api/v1/stats/catalog` already publishes.
  `Page` exists because the API signal carries no bot filtering and this host's
  traffic is mostly crawlers: it is the only one of the two safe to rank a public
  list on (`internal/engage/socialdigest`).
- `RotatedFiles(dir, base)` / `LogFile.Open()` — lists rotated files (skips the live
  `access.log`) and opens gzip transparently. `LogFile` is just a path; the worker's
  cursor key across numeric-suffix rotation is an FNV-64 hash over the file's
  decompressed content, computed by the worker itself.

## Conventions

- **Dict/heuristic only, no external calls.** Pure functions over strings + files.
- **Bot list is deliberately small** (`bot.go`): missed bots only inflate a
  transparency number; over-aggressive matching would drop real people.
- **Semantics live here, not in SQL.** The worker's queries are additive plumbing;
  what counts as a view and how it dedups is defined in this package.
