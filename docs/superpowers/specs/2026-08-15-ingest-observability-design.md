# Pipeline observability: queue-depth metrics, dashboard, alerts

Date: 2026-08-15
Status: approved, not implemented

## Why

On 2026-08-15 the site served a job feed whose newest posting was six hours old, while
Postgres was receiving new jobs every minute. The cause: `freehire-reindexw.service`
started at 14:14:47, its `ExecStartPre` stopped `freehire-search-drain.timer` (by design —
both write the same Meilisearch index), and the run then spent five hours in its
Postgres-side aggregator-duplicate pass without ever reaching the Meili push. The drain
stayed off for the whole window, `search_outbox` grew to ~42k entries, and the live index
went stale.

Nothing alerted. The signal was already in Prometheus the entire time:

```promql
time() - freehire_worker_last_run_timestamp_seconds{exported_job="search-drain"}
```

read 18,055 seconds when the incident was found by hand. No rule watched it.

This change closes the gap the `prod-observability-infra` change in `freehire-ops`
explicitly deferred:

> Exposing the application's own queue-depth/batch-timing metrics (which is what would
> have caught tonight's issue fastest) is a natural follow-on, not part of this change.

## What already exists

Do not rebuild any of this:

| Component | Location |
|---|---|
| Prometheus | `litellm-prometheus-1` on `204.168.137.149`, scrape interval 15s |
| Grafana | `litellm-grafana-1`, published at `grafana.freehire.me` behind an nginx IP allowlist |
| Scrape jobs for host-2 | `host2-node` (:9100), `host2-postgres` (:9187), `hire-api` (:9091 blue, :9092 green), `meilisearch` (:9200) |
| Per-run worker metrics | `internal/worker/metrics.go` writes `freehire_worker_last_run_{timestamp_seconds,duration_seconds,success}` into `PROM_TEXTFILE_DIR`; 229 series are live |
| Alert rules, versioned | `freehire-ops/provision/litellm-host/grafana/provisioning/alerting/rules.yaml`, group `host2-infra`, folder `Infra Alerts` |
| Telegram contact point | `telegram-ops`, already receiving the five infra rules |

### Label trap

node_exporter's textfile collector does not use `honor_labels`. A worker writes
`job="search-drain"`, Prometheus keeps its own `job="host2-node"` and renames the
exposed one. **Every query against worker metrics must use `exported_job` and
`exported_instance`.** Querying `job=` returns an empty vector silently.

## What is missing

Queue depth. Nothing exports `search_outbox`, `enrichment_outbox`, or `semantic_outbox`.
Worker-run age answers "is the worker alive"; it cannot answer "is the worker keeping up".

Measured 2026-08-15 19:20 UTC, mid-incident:

| Queue | Rows | Oldest entry |
|---|---|---|
| `search_outbox` | 41,858 | 2026-08-07 |
| `semantic_outbox` | 781,455 | 2026-08-09 |
| `enrichment_outbox` | 1,049,297 | 2026-06-12 |
| `board_health` | 7,002 failing / 1,882 cooled of 83,778 | — |

## Architecture

### Approach: a run-once worker writing a textfile

`cmd/queue-metrics` runs on a one-minute systemd timer, queries the depths, and writes a
single `.prom` file into the existing `PROM_TEXTFILE_DIR`. node_exporter picks it up on
its own schedule; Prometheus scrapes node_exporter as it already does.

Rejected alternatives:

- **A custom collector on `cmd/server`'s existing `/metrics` listener**
  (`internal/observability/observability.go:68`). Cheapest to wire, but prod runs blue and
  green simultaneously, so every database-wide gauge is emitted twice and every query and
  alert rule grows a permanent `max by(...)` wrapper. Worse, the collector would run per
  scrape: 15s × 2 colours is eight `COUNT(*)` passes per minute over tables of ~1M rows,
  with the frequency governed by `scrape_interval` — a parameter that lives in another
  repository and is tuned by someone not thinking about Postgres load. A slow database
  would then also time out the scrape and take the runtime metrics down with it.
- **`postgres_exporter` custom queries.** No Go at all, but the SQL would live outside
  this repository with no tests and no compile-time tie to the schema. Silent drift is
  the normal failure mode of that pattern.

The worker keeps the cost fixed at one pass per minute and puts the SQL under sqlc and
under test.

### Changes in `hire`

```
cmd/queue-metrics/main.go        new run-once worker
internal/db/queries/metrics.sql  the aggregate queries
internal/worker/metrics.go       extract the atomic write into an exported helper
```

`internal/worker/metrics.go:66-74` already implements the write-then-rename that keeps
node_exporter from reading a half-written file. Extract it as
`worker.WriteTextfile(dir, name, body string) error` and have `writeRunMetrics` call it too,
rather than duplicating the pattern in the new worker.

`cmd/queue-metrics` follows the standard shape from `internal/worker/AGENTS.md`:
`worker.Main(run)`, `worker.Bootstrap`, `worker.ExitCode`. Because `worker.Main` already
calls `writeRunMetrics` (`internal/worker/main.go:30`), the new worker publishes its own
`freehire_worker_last_run_*{exported_job="queue-metrics"}` series for free — which alert
rule 7 below depends on.

### Metrics published

```
freehire_queue_depth{queue="search_outbox|enrichment_outbox|semantic_outbox"}
freehire_queue_dead_letters{queue="..."}
freehire_queue_oldest_age_seconds{queue="..."}
freehire_boards_total{state="healthy|failing|cooled"}
freehire_catalogue_newest_job_timestamp_seconds
```

All three outbox tables share one shape — `(id, job_id, attempts, claimed_at, failed_at,
last_error, created_at)` — and delete rows on success, so depth is
`count(*) WHERE failed_at IS NULL`, dead letters are `count(*) WHERE failed_at IS NOT
NULL`, and oldest age is `now() - min(created_at)`. All three aggregates come from one
pass per table; Postgres scans the whole table either way, so the second and third
aggregate are free.

### Changes in `freehire-ops`

```
provision/host2/systemd/freehire-queue-metrics.service
provision/host2/systemd/freehire-queue-metrics.timer          OnUnitActiveSec=1min
provision/litellm-host/grafana/provisioning/dashboards/provider.yaml
provision/litellm-host/grafana/provisioning/dashboards/freehire-pipeline.json
provision/litellm-host/grafana/provisioning/alerting/rules.yaml   new group
```

The Grafana container already mounts `./grafana/provisioning` in full, so adding a
`dashboards/` subdirectory needs no compose change.

## Dashboard

One dashboard, `Freehire Pipeline`, four rows ordered from user-visible symptom down to
the thing to fix:

1. **Freshness** — age of the newest job in Postgres; age of the oldest `search_outbox`
   entry; time since the last `search-drain` and `reindex` run.
2. **Queues** — depth of all three outboxes on a log scale, dead letters, oldest-entry
   age.
3. **Board fleet** — healthy / failing / cooled over time, stacked.

   No per-board table, though an earlier draft of this design assumed one. Naming the
   worst boards from Prometheus would mean a series per board, and the fleet holds ~84k
   of them — a cardinality disaster for data that is only ever read as a top-N list. The
   panel beside the graph carries the `board_health` SQL to run instead.
4. **Workers** — a table of worker → time since last run → last exit status, sorted by
   staleness. Built entirely on the 229 series that already exist.

The log scale on row 2 is not cosmetic: `search_outbox` belongs near zero while
`enrichment_outbox` sits near a million, and on a linear axis the one that matters is
pinned to the axis.

The dashboard JSON must reference datasource uid `bfuyturd0z9q8f`. That datasource was
created through the Grafana UI, not provisioned — there is no `provisioning/datasources/`
directory. The existing alert rules already carry the same dependency, so this adds no
new fragility, but a Grafana rebuild would blank the panels.

## Alert rules

New group `freehire-pipeline` in folder `Pipeline Alerts`, all routed to `telegram-ops`.

| # | Rule | Expression | `for` | Severity |
|---|---|---|---|---|
| 1 | search-drain stalled | `time() - freehire_worker_last_run_timestamp_seconds{exported_job="search-drain"} > 900` | 5m | critical |
| 2 | catalogue stopped growing | `time() - freehire_catalogue_newest_job_timestamp_seconds > 1800` | 10m | critical |
| 3 | reindex hung | `time() - freehire_worker_last_run_timestamp_seconds{exported_job="reindex"} > 32400` | 15m | critical |
| 4 | embed / enrich stalled | `time() - freehire_worker_last_run_timestamp_seconds{exported_job=~"embed\|enrich"} > 14400` | 15m | warning |
| 5 | search backlog not draining | `freehire_queue_depth{queue="search_outbox"} > 20000` | 30m | warning |
| 6 | board fleet degrading | `freehire_boards_total{state="cooled"} > 1.5 * (freehire_boards_total{state="cooled"} offset 24h)` | 1h | warning |
| 7 | exporter stopped running | `time() - freehire_worker_last_run_timestamp_seconds{exported_job="queue-metrics"} > 600` | 5m | warning |
| 8 | exporter running but failing | `freehire_worker_last_run_success{exported_job="queue-metrics"} == 0` | 10m | warning |

Rules 1-3 each catch the 2026-08-15 incident independently. Rule 1 would have fired at
14:30, fifteen minutes after the timer stopped.

Rule 3's threshold is three missed runs: `freehire-reindexw.timer` is
`OnCalendar=*-*-* 00/3:15:00`, so a three-hour cadence. A hung run never stamps a
completion, so last-run age covers "hung" and "not scheduled" with one expression.

Every rule on a queue metric sets `noDataState: Alerting`, not `NoData`. If the exporter
stops writing its file the series disappears, and a `NoData` rule would fall silent
exactly when it should shout.

Rules 7 and 8 cover the two distinct ways the exporter can stop being trustworthy, and
only together. If the timer stops, the file goes stale and no `.prom` is refreshed — rule
7. But if the timer keeps firing while the collection itself fails (an unreachable
database), the worker exits non-zero every minute, so its run-outcome file IS refreshed
with a current timestamp and rule 7 stays quiet — while `freehire-pipeline.prom` keeps
its last good content and Prometheus scrapes a flat line of stale gauges indefinitely.
Rule 8 is what catches that, reading the `success` gauge the failed run already
publishes.

The worker deliberately does NOT delete its file when a collection fails. Doing so would
turn a one-minute database blip into a vanished series and, via `noDataState: Alerting`,
into a page — while also discarding the last known-good reading, which is exactly what a
human looking at the graph during an incident wants to see. Reporting the failure and
leaving the last measurement in place is the honest split: the `success` gauge says the
number is stale, and the number itself stays readable.

### Deliberately not alerted

`freehire_queue_oldest_age_seconds{queue="search_outbox"}` is graphed but never
alerted, because it is structurally unbounded. `ClaimSearchOutboxBatch`
(`internal/db/queries/search_outbox.sql:53`) orders `job_posted_at DESC` — freshest
first — so entries for jobs with an old `posted_at` sink to the tail and are never
claimed while fresher work keeps arriving. The purge that would remove them,
`DeleteSearchOutboxCreatedBefore`, additionally requires `j.updated_at < before`
(lines 84-86), and ingest keeps touching those jobs, so their `updated_at` stays recent
and the purge skips them. 5,309 such rows were measured, all with `attempts = 0` — never
even claimed once. See "Known defect" below.

Depth alerts on `enrichment_outbox` and `semantic_outbox` are also omitted. Both are
structurally underwater — enrichment against a billing-capped LLM key pool, embedding
against bounded CPU — so any absolute threshold is permanently breached and would be
muted on day one. They are graphed only.

### Thresholds 5 and 6 are provisional

No honest baseline exists right now: the drain has been off for five hours and 41,858 is
not a steady state. Both thresholds must be re-tuned after one week of collected data.
This is a required follow-up step, not an aspiration.

## Testing

- `cmd/queue-metrics`: a golden test on the rendered `.prom` text — exact metric names,
  `# HELP` / `# TYPE` lines, and label sets. This is the contract the alert rules bind to.
- `internal/db`: an integration test (`//go:build integration`) covering the three
  aggregate queries against a seeded database, including the `failed_at` split.
- `internal/worker`: extend the existing `metrics_test.go` to cover `WriteTextfile`
  directly, keeping the atomic-rename assertions.

Per `CLAUDE.md`, run `go vet -tags=integration ./...` before pushing.

## Failure behaviour

A failed metrics collection must never be silent and must never be fatal to anything
else. The worker returns 1, which `worker.Main` records as
`freehire_worker_last_run_success = 0` for this binary — the gauge rule 8 reads.

It deliberately leaves its own `freehire-pipeline.prom` in place on failure rather than
deleting it, for the reasons under "Alert rules" above. The worker holds no locks and
writes nothing to the database, so a hung run cannot block ingest, drain, or reindex.

## Known defect, out of scope

The `search_outbox` tail described above is a real defect: 5,309 entries that no code
path will ever process. It follows from two individually reasonable decisions — index
the freshest first, and do not purge what changed after the reindex began — meeting at
an edge neither anticipated. It is pre-existing and unrelated to this change, and fixing
it here would widen the scope past observability into queue semantics. It should be
raised separately.

## Out of scope

- Fixing the `search_outbox` tail (above).
- Adding a `TimeoutStartSec` to `freehire-reindexw.service` so a hung run cannot hold
  the drain timer down indefinitely. This is the correct structural fix for the incident
  that motivated this change, but it is an ops change, not an observability one, and it
  belongs in `freehire-ops` on its own.
- Provisioning the Grafana datasource as a file.
- Alertmanager. Grafana's own alerting already reaches `telegram-ops`.
