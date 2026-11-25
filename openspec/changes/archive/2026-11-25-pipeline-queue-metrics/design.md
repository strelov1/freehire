## Context

The full investigation and the cross-repo design live in
`docs/superpowers/specs/2026-08-15-ingest-observability-design.md` (commit `42bfbfcf`).
This document covers only the decisions that shape the code in this repository.

Existing infrastructure, none of which this change rebuilds: Prometheus and Grafana run
on the litellm host and already scrape host-2's node_exporter, postgres_exporter, both
API colours, and Meilisearch. `internal/worker/metrics.go` already writes
`freehire_worker_last_run_{timestamp_seconds,duration_seconds,success}` into
`PROM_TEXTFILE_DIR`, and 229 such series are live. What is absent is any measure of how
much work is waiting.

## Goals / Non-Goals

**Goals:**

- Publish queue depth, dead letters, oldest-entry age, board-fleet state, and catalogue
  freshness as Prometheus gauges.
- Keep the measurement cost fixed and independent of how often anything scrapes.
- Make the metric names and label sets a stable contract for the `freehire-ops`
  dashboard and alert rules.

**Non-Goals:**

- The dashboard, the alert rules, and the systemd unit and timer. Host configuration,
  delivered in `freehire-ops`.
- Fixing the `search_outbox` tail described under Risks. Pre-existing, unrelated, and a
  change to queue semantics rather than to observability.
- Adding `TimeoutStartSec` to `freehire-reindexw.service`. The correct structural fix for
  the incident that motivated this change, but an ops change.
- Alertmanager. Grafana's own alerting already reaches the existing Telegram contact
  point.

## Decisions

### A run-once worker, not a collector on the API's `/metrics`

`cmd/server` already runs a Prometheus listener
(`internal/observability/observability.go:68`), so registering a custom collector there
would need no new unit. Rejected on three counts.

Production runs blue and green simultaneously, so every database-wide gauge would be
emitted twice, and every panel query and alert rule would carry a permanent `max by(...)`
wrapper to compensate.

A collector runs per scrape. At a 15-second interval across two colours that is eight
`COUNT(*)` passes per minute over tables holding roughly a million rows, with the
frequency governed by `scrape_interval` — a value that lives in another repository and is
tuned by someone not reasoning about Postgres load.

A slow database would then also time out the scrape, taking the runtime metrics down
alongside the queue metrics, exactly when both are wanted.

A worker on its own timer decouples measurement frequency from scrape frequency, which is
the same reason the existing per-run worker metrics go through the textfile collector
rather than a Pushgateway.

Also rejected: `postgres_exporter` custom queries. No Go at all, but the SQL would sit
outside this repository with no tests and no compile-time relationship to the schema, and
silent drift is that pattern's normal failure mode.

### Extract the atomic write rather than reimplement it

`internal/worker/metrics.go:66-74` already writes to a `.tmp` path and renames it into
place, so the textfile collector never reads a half-written file. The new worker needs
the identical guarantee for a different filename.

Extract it as `worker.WriteTextfile(dir, name, body string) error` and have the existing
`writeRunMetrics` call it. Duplicating a subtle atomicity guard in a second file is how
one of the two copies eventually loses it.

### One pass per table, three aggregates

All three outbox tables share a shape — `(id, job_id, attempts, claimed_at, failed_at,
last_error, created_at)` — and delete rows on success. Depth, dead letters, and oldest
age therefore come from one scan per table: Postgres reads the whole table for the count
regardless, so the other two aggregates cost nothing extra.

The three queries stay separate and explicit rather than parameterising the table name.
sqlc generates from static SQL, and a dynamic table name would defeat both codegen and
the schema's compile-time guarantee.

### Zero and absent mean different things

An empty queue publishes `0`. An empty catalogue publishes nothing.

A drained queue is a real, healthy measurement, and a rule watching it must be able to
distinguish "drained" from "exporter died" — so the series must exist. Catalogue
freshness is a timestamp, and `0` would read as 1970, i.e. infinite staleness; an empty
catalogue is a fresh-install state rather than an incident, so no value is the honest
answer. The consuming alert rules set `noDataState: Alerting` to cover the absent case.

### Metric naming

`freehire_queue_depth`, `freehire_queue_dead_letters`, `freehire_queue_oldest_age_seconds`,
`freehire_boards_total`, `freehire_catalogue_newest_job_timestamp_seconds` — the
`freehire_` prefix matches the existing series, and the queue is a label rather than part
of the name so one panel and one rule can span all three outboxes.

A trap for anyone writing queries against these: node_exporter's textfile collector does
not set `honor_labels`, so a `job` label written by a worker is renamed to `exported_job`
on scrape. Queries against the per-run worker metrics must use `exported_job`; querying
`job` returns an empty vector with no error. The metrics added here use `queue` and
`state` labels precisely to sidestep that collision.

## Risks / Trade-offs

**A per-minute pass over `enrichment_outbox` (~1M rows) and `semantic_outbox` (~780k)
adds steady read load** → The pass is read-only, takes no locks, and is bounded to once
per minute regardless of scrape activity. If it proves measurable against host-2's
existing I/O pressure, the timer interval is the single tuning knob and lives in
`freehire-ops`.

**`freehire_queue_oldest_age_seconds` for `search_outbox` reads implausibly high, so it
looks like a broken metric** → It is published and graphed but never alerted on. The
metric was honest; the queue was not.

*Post-ship correction.* This paragraph originally blamed `ClaimSearchOutboxBatch`'s
`job_posted_at DESC` ordering for starving old postings. Checked against prod afterwards
and that was wrong: of the entries older than a day, **zero** belonged to an open
canonical job — 631 sat behind a `duplicate_of` and 138 behind a closed job. The claim
skips exactly those, correctly, and nothing deleted them;
`DeleteSearchOutboxCreatedBefore` also requires `jobs.updated_at` to predate the run, and
a job demoted to a duplicate stays in its board's feed so ingest keeps touching it.
Fixed by `DeleteIneligibleSearchOutbox`, which the drain runs before each pass.

**The metric names become a contract with a repository that cannot enforce it** → A
golden test over the rendered text pins the exact names, label sets, and `# HELP`/`# TYPE`
lines, so renaming one is a deliberate, visible edit rather than a silent breakage of
someone else's alert rule.

**Depth thresholds cannot be calibrated from current data** → Alert thresholds live in
`freehire-ops` and are explicitly provisional; the observed 41,858 backlog is an active
incident, not a baseline. Re-tuning after a week of collected data is a required step
there, not here.

### A failed collection leaves the last file in place

The worker returns 1 on a query failure, which `worker.Main` records as
`freehire_worker_last_run_success = 0`. It does NOT delete its own
`freehire-pipeline.prom`.

Deleting would turn a one-minute database blip into a vanished series and, under the
consumers' `noDataState: Alerting`, into a page — while discarding the last known-good
reading, which is exactly what a human looking at the graph during an incident wants.
The honest split is that the `success` gauge reports the numbers are stale while the
numbers stay readable.

The consequence for `freehire-ops`: the alert set needs BOTH a run-age rule and a
run-success rule. A stopped timer refreshes nothing, so run age catches it. A firing
timer whose collection keeps failing DOES refresh the run-outcome file with a current
timestamp, so run age stays quiet while the queue gauges sit frozen — only the `success`
gauge catches that one.

## Migration Plan

The worker is additive and writes nothing. Deployment order: merge and deploy the binary
first, then add the unit and timer in `freehire-ops`, then the dashboard, then the alert
rules. Rollback is stopping the timer; no data is left behind beyond one stale `.prom`
file, which the companion alert on the exporter's own run age surfaces.

## Open Questions

None blocking. The timer interval and the alert thresholds are deliberately deferred to
`freehire-ops`, where they can be tuned without a code change.
