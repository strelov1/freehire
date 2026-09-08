## Why

`internal/search/search/AGENTS.md` documents, by hand, a deploy-ordering hazard for
both the jobs and companies Meilisearch indexes: a binary rolled out before its
settings patch (a sortable/filterable attribute, the skill embedder) reaches the LIVE
index turns every request for it into a Meili 400, which this package's error mapping
turns into a 500 for every caller of the affected sort, filter, or match ranking — not
just the one who picked the new value, since a settings patch replaces
`sortableAttributes`/`filterableAttributes` wholesale. The doc's own words for the gap:
"There is no operator script for this." Today the only way this drift is discovered is
a caller hitting the 500 first.

## What Changes

- `internal/search/search` gains `Client.SettingsDrift(ctx)`, which fetches the live
  jobs and companies indexes' settings and reports every sortable attribute, filterable
  attribute, and embedder this binary's own `facetSettings()`/`companySettings()`
  expect that the live index has not yet declared. The comparison is pure
  (`diffSettings`) and unit-tested against fabricated settings; `SettingsDrift` itself
  is integration-tested against a real engine (testcontainers).
- New worker `cmd/search-settings-drift`: calls `SettingsDrift` on a schedule and
  publishes the gap count (`freehire_search_settings_drift_count`) via the
  node_exporter textfile collector, mirroring `cmd/llm-probe`'s and
  `cmd/queue-metrics`' shape (no-op without `PROM_TEXTFILE_DIR`, no-op without
  `MEILI_MASTER_KEY`, never exits non-zero for a gap it only observed). Systemd
  service/timer added under `deploy/systemd/`, every 5 minutes.
- `internal/search/search/AGENTS.md` and the root `AGENTS.md` worker list updated to
  describe the new worker and retire the "no operator script for this" line it closes.

This worker does not fix the hazard — settings-before-binary stays a human decision
made when sequencing a deploy and a reindex. It makes an already-drifted state visible
on a schedule instead of by a caller hitting the 500 first.

## Capabilities

### New Capabilities

- `search-settings-observability`: reporting drift between what a binary's Meilisearch
  client code expects the live jobs/companies index settings to declare (sortable
  attributes, filterable attributes, embedders) and what the live index actually
  declares, on a schedule, via a Prometheus gauge.

## Impact

- `internal/search/search/settings_drift.go` (new) — `Client.SettingsDrift`, the pure
  `diffSettings` comparison.
- `internal/search/search/settings_drift_test.go`, `settings_drift_integration_test.go`
  (new) — unit coverage for the comparison, integration coverage against a real engine.
- `cmd/search-settings-drift/` (new) — the worker: `main.go`, `render.go`, tests.
- `deploy/systemd/freehire-search-settings-drift.{service,timer}` (new) — recorded here
  per `deploy/AGENTS.md`'s convention; installing them on the host is a separate,
  manual step, same as every other worker addition.
- `.gitignore` — the new binary target.
- `internal/search/search/AGENTS.md`, root `AGENTS.md` — documentation.
- No schema change, no change to any existing read or write path — this is a new,
  read-only, additive worker.
