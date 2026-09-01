# Publish per-provider ingest freshness

## Why

The residential proxy's credentials stopped working on 2026-08-18. Every provider
behind it — eightfold, hh, djinni, wantedkr, vagas, peopleforce, wantapply,
cleverstaff, geekjob, 2gis — returned `407 Proxy Authentication Required` on every
request from that moment. It was found by hand on 2026-08-31, **thirteen days later**,
while looking at something else. ~54,000 open postings had gone unverified the whole
time.

Nothing alerted, and the reason is structural: none of the three signals the fleet
publishes measures data arriving per provider.

| Published today | Why it stayed quiet |
|---|---|
| `freehire_catalogue_newest_job_timestamp_seconds` | Catalogue-wide. Greenhouse and Workday kept adding postings every minute, so the gauge never aged. |
| `freehire_boards_total{state=...}` | Fleet-wide. A provider going 0% → 100% failing moves 92 boards out of 97,221 — inside the normal 4.9% failing background. |
| `freehire_worker_last_run_*` | Measures the *process*, not the *data*. The peopleforce run on 2026-08-31 skipped all 92 boards as cooled, reported `failed=0`, and exited 0 — indistinguishable from a run that ingested everything. |

Querying `board_health` for the missing signal takes one line and exposes more than the
proxy outage:

| Provider | Last successful crawl | Silent for |
|---|---|---:|
| `gulftalent`, `bayt` | never | — |
| `recruitingsolutions` | 2026-07-14 | **49 days** |
| `careerspage` | 2026-07-18 | **44 days** |
| `alfabank` | 2026-07-29 | **34 days** |
| `functionalworks` | 2026-08-12 | 19 days |
| the proxied cluster | 2026-08-18 | 13 days |

Six providers beyond the proxy outage have been dead for weeks, two have never
succeeded at all, and none of it was visible.

## What Changes

- `cmd/queue-metrics` publishes one new family:
  `freehire_provider_last_success_timestamp_seconds{provider="<name>"}` — the most recent
  successful crawl across all of that provider's boards.
- A provider whose every board has never succeeded publishes **no sample** rather than a
  zero, matching how `freehire_catalogue_newest_job_timestamp_seconds` already treats an
  empty catalogue: an absent series is what the consuming alert rule reads as "no data",
  and 1970 would read as "infinitely overdue" — the same claim made much more loudly than
  the evidence supports.
- The alert rule itself lives in `freehire-ops` and is out of this repository's reach;
  the proposal names it so shipping the gauge is not mistaken for shipping the alert.

## Impact

- Affected specs: `ingest-board-health` (a new requirement beside "Unhealthy boards are
  visible without grepping logs")
- Affected code: `internal/platform/db/queries/metrics.sql` (one query, then `make sqlc`),
  `cmd/queue-metrics/collect.go`, `cmd/queue-metrics/render.go`
- No migration. `board_health.last_success_at` already exists and is already written on
  every successful crawl.

## The query this deliberately does NOT use

The obvious source is the jobs table — `SELECT source, max(last_seen_at) FROM jobs WHERE
closed_at IS NULL GROUP BY source`. Measured on prod it is a **41-second** parallel
sequential scan reading 2.1M buffers, and `metrics.sql`'s own header states that every
query in it "runs once a minute alongside ingest, search-drain, and reindex, and must
never be the reason one of them waits". On a host already documented as disk-bound, that
query once a minute would evict the buffer cache the rest of the pipeline depends on.

The same measurement over `board_health` — 97,221 rows against the jobs table's 8M —
runs in **54ms**, and is the better signal anyway: it reports whether the crawl
succeeded, not whether a posting happened to change.

## Out of scope

- The alert rule and dashboard panel (`freehire-ops`).
- Fixing any of the providers this reveals. `gulftalent` and `bayt` have never succeeded;
  `recruitingsolutions`, `careerspage`, `alfabank` and `functionalworks` have been dead
  for weeks. Each is its own investigation — this change makes them visible.
- Reporting ingest failures to Sentry. A board failing is ordinary (boards close every
  day), so per-failure reporting would be noise; the signal that matters is duration,
  which is a gauge and an alert rule, not an exception.
