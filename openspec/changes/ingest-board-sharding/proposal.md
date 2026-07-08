## Why

`sources/workday.yml` holds ~6165 boards, and Workday's shared infra aggressively rate-limits (HTTP 429). Retrying each throttled board burns the clock, so the hourly run only reaches ~1700 boards before the 40-minute systemd timeout kills it — the same first slice every run, leaving ~72% of Workday boards never crawled. A provider's board list can outgrow a single timed run.

## What Changes

- `cmd/ingest` accepts an optional `--shard=i/n` flag (or `SHARD` env): it crawls only a round-robin 1/n slice of the board file. N staggered timers then partition a huge file across several runs, each finishing within its timeout, while the whole file is covered over one cycle.
- The slice is applied after validation (the full file is still validated on every shard's run) and before the crawl; the existing per-company stale-job sweep already scopes closes to boards actually crawled, so a partial shard run stays safe.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `source-ingest`: the standalone ingest command additionally accepts a shard selector that restricts a run to a round-robin slice of the board file.

## Impact

- `cmd/ingest/main.go` (flag/env parsing + slice application), `internal/sources/shard.go` (`ParseShard`, `Config.Shard`).
- Ops (`freehire-ops`): replace the single hourly `freehire-ingest@workday` timer with 6 staggered shard timers (`--shard=1/6`..`6/6`, every 6h). No DB migration, no code change to the pipeline or sweep.
