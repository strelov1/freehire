## 1. Comparison

- [x] 1.1 Add pure `diffSettings(indexUID string, live, want *meilisearch.Settings) []string` to `internal/search/search/settings_drift.go`, comparing sortable attributes, filterable attributes, and embedders, one-directional (only "expected but not live" is a gap).
- [x] 1.2 Add `Client.SettingsDrift(ctx)` that fetches the live jobs and companies indexes' settings via `GetSettingsWithContext` and diffs each against `facetSettings()`/`companySettings()`.

## 2. Tests

- [x] 2.1 Unit-test `diffSettings` against fabricated settings: match (nil result), missing sortable, missing filterable, missing embedder, and the one-directional rule (a live-only attribute is not reported).
- [x] 2.2 Integration-test `SettingsDrift` against a real engine (testcontainers): empty after both indexes are ensured; non-empty for a freshly created, never-settings-applied index. Run and confirm both pass against Docker.

## 3. Worker

- [x] 3.1 `cmd/search-settings-drift/main.go`: gate on `PROM_TEXTFILE_DIR` then `MEILI_MASTER_KEY` (both no-ops, exit 0), call `SettingsDrift`, publish the gauge, log every gap found, never exit non-zero for an observed gap (only for a failure to observe).
- [x] 3.2 `cmd/search-settings-drift/render.go`: `freehire_search_settings_drift_count` gauge with HELP/TYPE.
- [x] 3.3 Tests: textfile-name collision guard (mirrors `queue-metrics`/`llm-probe`), no-op without `PROM_TEXTFILE_DIR`, no-op without `MEILI_MASTER_KEY`, render publishes zero and a nonzero count with HELP/TYPE.

## 4. Deployment record

- [x] 4.1 `deploy/systemd/freehire-search-settings-drift.service` and `.timer`, mirroring `freehire-llm-probe`'s shape (lowest CPU/IO weight, 5-minute monotonic timer, not persistent).
- [x] 4.2 Add `/search-settings-drift` to `.gitignore`.

## 5. Documentation

- [x] 5.1 `internal/search/search/AGENTS.md`: describe the new worker under "Adding a sortable attribute", replacing the "no operator script for this" line.
- [x] 5.2 Root `AGENTS.md` worker list: add the `search-settings-drift` entry.

## 6. Verification

- [x] 6.1 `gofmt -l .` clean (repo-wide, excluding node_modules).
- [x] 6.2 `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` all clean.
- [x] 6.3 `go test ./...` (whole repo) green, 0 FAIL — including `TestEveryCmdBinaryIsGitignored`.
- [x] 6.4 `pnpm check:links` clean.
