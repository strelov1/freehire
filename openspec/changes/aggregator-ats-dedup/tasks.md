## 1. Aggregator provider set

- [ ] 1.1 Add `sources.AggregatorProviders() []string` returning the registry's
  `aggregator()`-marked providers; unit-test that it covers the marked sources and
  excludes a non-aggregator (ATS) provider.

## 2. Suppression query

- [ ] 2.1 Write `SuppressAggregatorDuplicatesForCompany` in
  `internal/db/queries/jobs.sql`: per `company_slug` + aggregator-provider `text[]` param,
  set `duplicate_of` on each open aggregator row to the id of an open canonical ATS row
  (`duplicate_of IS NULL`) of the same company, equal normalized title, and compatible
  country (`countries` overlap OR either empty); write only where `IS DISTINCT FROM` the
  current value. Regenerate sqlc (`make sqlc`).
- [ ] 2.2 Integration test (build-tag `integration`, testcontainers) covering: aggregator
  copy suppressed to the ATS row; ATS row stays canonical; same-title different-country not
  suppressed; ATS row never demoted; two aggregators without an ATS twin untouched;
  no-change run writes zero rows; closed ATS twin releases the aggregator copy.

## 3. Ingest wiring

- [ ] 3.1 In `cmd/ingest`, run the suppression pass per company immediately after
  `RecomputeRoleDuplicatesForCompany` (repost-collapse first, then cross-source
  suppression), passing `sources.AggregatorProviders()`; log a per-run count of
  suppressed/released rows, log-and-continue per company on error (mirroring the existing
  recompute pass).

## 4. Verification

- [ ] 4.1 On a scratch DB (or the ingest integration harness), run a crawl of one
  aggregator board plus its ATS twin and confirm the aggregator copy gets `duplicate_of`
  set, disappears from `ListJobs`, and is not enqueued for embedding/enrichment, while its
  detail-by-slug still resolves.
