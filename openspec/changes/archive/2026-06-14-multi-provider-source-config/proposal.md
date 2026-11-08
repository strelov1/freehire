## Why

Every single-source adapter (vk, ozon, sber, yandex, …) needs its own one-line board
file (`vk.yml` is literally `- company: VK`) and its own cron entry, because the loader
derives the provider from the file name. That is 14 one-line files and 14 cron lines for
sources that are each one company. Letting a board file carry the provider per entry — as
the spec already says each entry does — collapses them into a single `custom.yml`, while
multi-tenant board lists (greenhouse, gem) keep their own files.

## What Changes

- A board entry MAY name its own `provider`; when present it wins, otherwise the provider
  falls back to the file name (so existing per-provider files are unchanged). This makes
  one file able to list entries for several providers.
- Add `sources/custom.yml` holding the small single-source configs: the 13 boardless
  single-company providers plus yandex (its two `board: ru` / `board: com` entries). Delete
  their 14 one-line files. gem stays its own file (a ~70-company board list).
- Validation runs per entry against the registry by that entry's resolved provider.
- The post-run unseen-job sweep runs **per provider within a run**: only a provider that
  ingested at least one job has its stale jobs closed, so one provider's crawl failure in
  a shared run cannot mass-close another provider's catalogue. `pipeline.Runner.Run`
  returns per-provider stats to make this possible; the crawl stays one bounded
  concurrent pool (a slow self-pacing provider like vk occupies one slot, never blocking
  the others).

No change to adapters, the normalized write path, the DB schema, or the cron mechanism
(one new `ingest sources/custom.yml` line replaces the 14 it removes).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities

- `source-ingest`: a board entry's `provider` may be set per entry (file name is the
  fallback default), so one file can list multiple providers.
- `job-lifecycle`: the unseen-job sweep is scoped and guarded per provider within a run.

## Impact

- `internal/sources/config.go` (per-entry provider resolution, per-entry validation),
  `internal/pipeline/pipeline.go` (`Run` returns `map[string]Stats`), `cmd/ingest/main.go`
  (per-provider sweep loop), `sources/custom.yml` (new), 14 deleted one-line files.
- Spec deltas: `source-ingest`, `job-lifecycle`.
- No DB migration, no API change. Verification: `go build/vet/test ./...` green.
