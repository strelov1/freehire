## Why

`cmd/ingest` writes every posting an aggregator adapter (Himalayas, EchoJobs, RemoteOK, HH,
~103 providers per `sources.AggregatorProviders`) fetches, even for a company that already
has an open posting from a non-aggregator source. A separate `cmd/reindex` pass
(`aggregator-ats-dedup`) later marks the aggregator copy `duplicate_of` the ATS row and hides
it from search, but the write, its search-outbox entry, and its index churn already happened
for nothing. A prod measurement (2026-08-13) found 57% of Himalayas' 18,580 distinct
companies have no non-aggregator coverage at all — the other 43% (~8,000 companies) are pure
write waste for Himalayas alone, repeated across every other aggregator.

## What Changes

- Add an ingest-time gate: before saving a posting from an aggregator-classified provider,
  skip the write if the company (exact `company_slug` match — NOT the folded match
  `aggregator-ats-dedup` uses; see design.md's "Coverage definition") already has an open
  posting from a non-aggregator source.
- The coverage check is backed by a live Meili lookup (`company_slug` + `source` are already
  filterable on the `jobs` index), batched once per board fetch — not a new Postgres table or
  migration.
- A new optional `Runner.Coverage` port in `internal/pipeline`, following the existing
  `BoardHealth`/`Closer`/`Toucher` pattern: nil disables the gate entirely (ATS board files,
  test fakes, an environment without Meili configured).
- A new `Stats.ATSCovered` counter and per-board log line, kept separate from the existing
  `Rejected` (non-technical) counter.
- `cmd/ingest` gains a new optional dependency on Meili (gated on `MEILI_MASTER_KEY` being
  set, the same convention `cmd/server` already uses — NOT `MEILI_URL`, which always has a
  default and so can't signal "is search configured"), needed only for aggregator-provider
  board files.

The existing `cmd/reindex` suppression pass (`aggregator-ats-dedup`) is unchanged and stays
as the correctness backstop for races this ingest-time gate does not close (see design.md).

## Capabilities

### New Capabilities
- `aggregator-ats-coverage-skip`: an ingest-time, company-level gate that skips writing an
  aggregator posting when the company already has open coverage from any non-aggregator
  source.

### Modified Capabilities
(none — `aggregator-ats-dedup`'s requirements are unchanged; this change adds a new,
independent gate earlier in the pipeline rather than altering the reindex pass)

## Impact

- `internal/pipeline/pipeline.go`: new `CoverageLookup` interface, `Runner.Coverage` field,
  threading through `ingestFetched`/`saveOne`, new `Stats` field.
- `internal/search`: new adapter implementing `CoverageLookup` against the Meili client.
- `cmd/ingest/main.go`: wire the adapter when `MEILI_MASTER_KEY` is set.
- Ops: confirm `MEILI_MASTER_KEY` reaches the aggregator-provider ingest systemd units — it
  may already be fleet-wide (four other workers require it) rather than needing to be added
  (out of scope for the code change itself, called out for deploy — see design.md's Migration
  Plan).
- No schema/migration changes.
