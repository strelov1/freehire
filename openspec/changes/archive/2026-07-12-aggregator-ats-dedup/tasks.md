## 1. Aggregator provider set

- [x] 1.1 Add `sources.AggregatorProviders(reg) []string` returning the registry's
  `aggregator()`-marked providers; unit-test that it covers the marked sources and
  excludes a non-aggregator (ATS) provider.

## 2. Suppression query

- [x] 2.1 Write `CompaniesWithAggregatorPostings` (driver) +
  `SuppressAggregatorDuplicatesForCompany` in `internal/db/queries/jobs.sql`: per
  `company` + aggregator-provider `text[]` param, set `duplicate_of` on each candidate
  open aggregator row to the id of an open canonical ATS row (`duplicate_of IS NULL`) of
  the same company, equal normalized title, and compatible country (`countries` overlap OR
  either empty); write only where `IS DISTINCT FROM` the current value. Regenerate sqlc
  (`make sqlc`).
- [x] 2.2 Integration test (build-tag `integration`, testcontainers) covering: aggregator
  copy suppressed to the ATS row; ATS row stays canonical; empty country still matches;
  same-title different-country not suppressed; ATS row never demoted; two aggregators
  without an ATS twin untouched; closed ATS twin releases the aggregator copy; driver
  returns only aggregator companies.

## 3. Reindex wiring

- [x] 3.1 In `cmd/reindex`, run the suppression pass per company immediately after
  `recomputeRoleDuplicates` (repost-collapse first, then cross-source suppression),
  passing `sources.AggregatorProviders(sources.All(nil))`; log a per-run count of
  re-marked rows, log-and-continue per company on error (mirroring the existing recompute
  pass).

## 4. Verification

- [x] 4.1 Integration test `TestSuppressedAggregator_HiddenFromListAndEnrichmentButServedBySlug`
  seeds one aggregator posting plus its ATS twin, runs the suppression pass, and confirms
  the aggregator copy gets `duplicate_of` set, is absent from `ListJobs`, is not enqueued
  by `EnqueuePendingJobs`, and still resolves via `GetJobBySlug`.
