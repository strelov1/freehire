## Context

`jobs.view_count` is a materialized counter bumped inside `RecordJobView`, only
when a signed-in user's first interaction with a job is an explicit view via the
authenticated `POST /api/v1/jobs/:slug/view` beacon. Anonymous web visitors and
external API consumers never touch that path, so both the card's "N views" and
`/open`'s "jobs viewed" (which today is `count(user_jobs WHERE viewed_at)`)
undercount reality by the overwhelming majority.

Key facts that shape the design (verified from code and the `freehire-ops` nginx
config):
- The job detail page is server-rendered: `GET /jobs/<slug>` triggers a SvelteKit
  SSR that fetches the backend. So every anonymous page open passes through the
  public nginx as a `/jobs/<slug>` line.
- The SSR→backend call uses `API_INTERNAL_URL` (e.g. `http://app:8080`),
  **bypassing public nginx**. So `/api/v1/jobs/<slug>` lines in the public log are
  external API consumers only — no SSR self-traffic, no double count.
- There is no `proxy_cache` today; nginx logs at distro defaults
  (`/var/log/nginx/access.log`, `combined` format, daily logrotate + gzip). The
  `combined` line carries client IP, the request line, status, and User-Agent —
  everything needed for path + IP+UA dedup + bot filtering.

## Goals / Non-Goals

**Goals:**
- Count anonymous + signed-in web + external API job views with **zero added work
  on the read path** — no per-request DB write, no in-handler side effect, so the
  read stays cheap and fully cacheable.
- Deduplicate to unique daily visitors so the number is defensible on a
  transparency page.
- Reuse the existing `jobs.view_count` column and card UI; widen its meaning
  rather than adding a parallel column.
- Backfill from existing log history so the number is meaningful on release.
- Yield per-job daily view trends as a byproduct for later insights use.

**Non-Goals:**
- Real-time / strongly-consistent view counts. Daily granularity is acceptable;
  a few miscounted views never matter.
- Per-user-accurate visitor identity or cross-device dedup. IP+UA is a documented
  approximation.
- Filtering bots on the API path (explicitly counted raw, by decision).
- The nginx `log_format`/`access_log` change and the systemd timer live in
  `freehire-ops`; this change specifies them but does not carry that code.

## Decisions

### D1: Aggregate offline from nginx logs, not in the app
The read path does nothing new. A daily cron worker parses the completed access
log and updates counters in batch.
- *Why over an in-app counter (in-memory batched or direct UPDATE):* any counting
  inside the Go handler forces every request to reach the app to be counted, which
  is incompatible with caching `GET /jobs/:slug` upstream. Logs are written even on
  cache hits and carry the real client IP/UA (lost at the SSR→backend boundary), so
  log aggregation is the only option that stays cache-friendly *and* can filter
  bots *and* covers anonymous + API uniformly.

### D2: Process whole rotated files; derive the day from each line; dedup per day
The worker processes each **rotated** (non-live) access-log file exactly once,
skipping the currently-written `access.log`. The default Debian/Ubuntu nginx
logrotate uses **numeric suffixes without dates** (`access.log.1`,
`access.log.2.gz`, …) and does not rotate exactly at midnight, so a file has no day
label and a calendar day can straddle two files. Therefore the day is taken from
each line's `[time_local]` timestamp (converted to UTC), and views are bucketed by
that day. The dedup key is `(hash(client-IP + User-Agent), slug, day)` — a visitor
counts once per job per UTC day.
- *Why derive the day from the line, not the filename:* filenames carry no date
  and shift on every rotation; the line timestamp is the only reliable day source
  and lets the worker run on the **current** log immediately (so backfill works now,
  before any ops format change). A future dated/JSON `access_log` in ops is a
  simplification, not a prerequisite.
- *Why not aggregate all files in one pass keyed by day:* that would need a `seen`
  set spanning the whole retained history (millions of `(visitor, slug, day)` keys
  during backfill). Per-file processing keeps memory bounded to one file; the only
  cost is a negligible double-count for a visitor active on a boundary day present
  in two files — acceptable for a transparency metric.

### D3: Cursor = a processed-file marker keyed by a content signature
`processed_view_logs(signature, filename, processed_at)` records which files are
done, keyed by a signature of the file's **decompressed content** (an FNV-64 hash
computed in the same read pass as aggregation). A signature already present is
skipped.
- *Why a content signature over inode or filename:* numeric-suffix rotation renames
  files (`.1` → `.2`) and later gzip creates a **new inode** for the same bytes, so
  neither the inode nor the filename is stable across a file's whole life. The
  content is — it is identical whether the day sits in `access.log.1` or
  `access.log.2.gz` — so hashing it makes a repeated `--backfill` (which reads the
  now-compressed history) recognize an already-applied file and skip it. Without
  this, the additive apply would silently re-inflate `view_count` on a re-run.
  A day marker cannot work either: one file spans two days and one day spans two files.
- *Why Postgres over a state file on host2:* survives worker redeploys, is shared
  and inspectable, and avoids fragile host-local disk state.

### D4: Two counted signals, asymmetric bot handling
- `GET /jobs/<slug>` (HTML page open, status 200): web views; skip requests whose
  User-Agent matches a small known-bot list.
- `GET /api/v1/jobs/<slug>` (status 200): API views; no bot filtering.
Both feed the same per-job daily unique count.

### D5: Redefine `view_count`; drop the POST bump
`view_count` becomes "distinct daily visitors across all traffic," maintained by
the worker as a running sum. `RecordJobView` drops its `bump` CTE; `POST /view`
still records `user_jobs.viewed_at` for per-user tracking (`/me/tracking`).
- *Why redefine rather than add `page_views`:* the card already renders
  `view_count` and signed-in users' page opens are already in the log, so the
  widened `view_count` already includes them — a parallel column would duplicate
  the same concept and split the UI.

### D6: Daily rollup table alongside the counter
Each processed file contributes per-`(day, slug)` uniques. These are applied with
an **additive** upsert into `job_daily_views(day, job_id, uniques)`
(`ON CONFLICT (day, job_id) DO UPDATE SET uniques = uniques + EXCLUDED.uniques`) and
the same per-day delta is added to `jobs.view_count`, in one batched statement per
file. Additivity is what makes a boundary day spanning two files sum correctly.
`/open`'s `viewed` switches to `SUM(jobs.view_count)`; the page is already cached
(60s module + 300s CDN) so the aggregate is cheap.

### D7: Backfill as a flag on the same worker
A `--backfill` run walks all retained rotated files, processing each unmarked file
via the same per-file path (aggregate → additive apply → mark file). It shares the
per-file idempotency, so it composes with ongoing daily runs and can run on the
**current** numeric-suffix logs immediately, before any ops format change.

## Risks / Trade-offs

- **IP+UA dedup is approximate** (NAT collapses distinct users to one IP →
  undercount; network change → recount next day) → Accepted and documented; a
  hashed visitor cookie in a custom JSON log could refine it later.
- **Bots inflate the web signal despite the UA skip** (crawlers spoof UAs) → Light
  known-bot list only; the transparency framing is "views," and gross inflation
  would be visible and can tighten the list.
- **Log format / path drift breaks the parser** → Parser targets the documented
  `combined` fields with a strict per-line match that skips unparseable lines;
  recommend a dedicated JSON `access_log` in ops for robustness. Confirm the live
  format on host2 before finalizing (prerequisite task).
- **Redefining `view_count` changes a public number** (some jobs jump up, none
  down) → It is additive and the direction is "more honest"; backfill lands the
  new baseline atomically on release.
- **Boundary-day double-count** (a visitor active on a day that straddles two
  rotated files counts once per file) → Accepted; bounded to at most one extra
  count per visitor per boundary day, negligible for a transparency metric, and the
  additive rollup keeps the day's total otherwise correct.
- **Worker gap on log-retention shortfall** (a file rotated away before processing)
  → Daily timer runs well within the multi-day retention; an unprocessed file is
  simply never counted (no correctness break), and backfill catches retained files.

## Migration Plan

1. Add migration: `job_daily_views`, `processed_view_logs` (keyed by file identity
   `(device, inode)`). (`view_count` column already exists.)
2. Ship the worker + queries; drop the `bump` CTE from `RecordJobView`; switch
   `GetEngagementStats.viewed` to `SUM(view_count)`.
3. Run `--backfill` once over the **current** retained numeric-suffix logs to seed
   the baseline — no ops change required for this.
4. Ops (follow-up, optional): a systemd timer running `rollup-views` daily under its
   own flock (not stacked with other rollups), with `VIEW_LOG_DIR` (default
   `/var/log/nginx`) and `VIEW_LOG_BASE` (default `access.log`) pointing at the log;
   run the timer's user with read access to the logs. Optionally add a dedicated
   dated/JSON `access_log` scoped to the job paths for robuster parsing — new files
   are processed once by inode, so switching format later needs no data migration.
5. Rollback: stop the timer; the read path and wire shape are unchanged, so the
   card/`/open` simply stop growing. `view_count` values remain valid.

## Open Questions

- Exact retained-history depth on host2 (bounds how far backfill can reach) — to
  confirm via the prerequisite host2 check.
- Whether to also emit the daily rollup to the existing insights surface now or
  defer until there's a concrete UI need (leaning defer).
