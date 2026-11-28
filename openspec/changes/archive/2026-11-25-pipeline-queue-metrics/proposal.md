## Why

On 2026-08-15 the live job feed went six hours stale while Postgres kept receiving new
jobs every minute: a `cmd/reindex` run stopped `freehire-search-drain.timer` (by design)
and then spent five hours in its Postgres-side dedup passes without ever releasing it, so
`search_outbox` grew to ~42k undrained entries. Nothing surfaced it — the existing
per-run worker metrics answer "is the worker alive", never "is the worker keeping up",
because no queue depth is exported at all. The `prod-observability-infra` change in
`freehire-ops` deferred exactly this as the natural follow-on.

## What Changes

- Add `cmd/queue-metrics`, a run-once-and-exit worker that measures the outbox queues,
  the ingest board fleet, and catalogue freshness, and publishes them as Prometheus
  gauges through the node_exporter textfile collector already wired up by
  `PROM_TEXTFILE_DIR`.
- Add the supporting aggregate queries to `internal/db/queries/`.
- Extract the existing atomic write-then-rename in `internal/worker/metrics.go` into an
  exported `worker.WriteTextfile`, so the new worker reuses it instead of duplicating the
  half-written-file guard. Pure refactor — no behavior change for existing callers.

Out of scope for this repository: the Grafana dashboard, the alert rules, and the
systemd unit and timer. Those are host configuration and live in `freehire-ops`
(`provision/host2/systemd/`, `provision/litellm-host/grafana/provisioning/`), delivered
as companion work.

## Capabilities

### New Capabilities
- `pipeline-metrics`: the ingest and indexing pipeline's queue depths, dead letters,
  board-fleet health, and catalogue freshness are published as Prometheus gauges, so a
  backlog that stops draining is observable rather than only discoverable by hand.

### Modified Capabilities

None. Extracting `worker.WriteTextfile` changes no requirement in `worker-lifecycle` —
the run-outcome metrics it already writes keep their exact names, labels, and semantics.

## Impact

- **New code**: `cmd/queue-metrics/`, queries in `internal/db/queries/metrics.sql`,
  regenerated `internal/db/` (via `make sqlc`).
- **Touched code**: `internal/worker/metrics.go` (extraction only).
- **Runtime**: one additional cron worker on host-2, one Postgres pass per outbox table
  per minute. No locks taken, no writes performed, so it cannot block ingest, drain, or
  reindex.
- **Config**: reuses the existing `PROM_TEXTFILE_DIR`; no new environment variable.
- **Downstream**: the metric names and label sets published here are the contract the
  `freehire-ops` dashboard and alert rules bind to.
