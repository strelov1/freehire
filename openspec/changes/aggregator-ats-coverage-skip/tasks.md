## 1. Pipeline port and stats

- [ ] 1.1 In `internal/pipeline/pipeline.go`, add the `CoverageLookup` interface (`NonAggregatorCompanies(ctx, companySlugs, aggregators []string) (map[string]bool, error)`) next to `BoardHealth`, and a new `Runner.Coverage CoverageLookup` field.
- [ ] 1.2 Add `Stats.ATSCovered int` and the matching `s.ATSCovered += o.ATSCovered` line in `Stats.add`.

## 2. Ingest-time gate

- [ ] 2.1 In `ingestFetched`, once per board (before the per-posting loop): when `r.Coverage != nil` and `sources.ProviderKind(sources.Taxonomy(), e.Provider) == sources.KindAggregator`, collect the distinct `company_slug` values from `raw` and resolve the covered set via `r.Coverage.NonAggregatorCompanies`. Thread the result down to `saveOne` as an explicit parameter (same style as the existing `rej`/`firstErr` threading — no new mutable Runner state); pass `nil` for every non-aggregator board or when `r.Coverage == nil`.
- [ ] 2.2 In `saveOne`, after the existing `outOfCatalogue` check and before `r.save`: if the covered-set parameter is non-nil and contains the posting's folded `company_slug`, skip the write, increment `Stats.ATSCovered`, and return — without touching `rej`/`Rejected`.
- [ ] 2.3 Add the per-board log line (mirroring `rejections.log`), emitted only when `ATSCovered > 0`: `"ingest: %s board %q (%s): skipped %d/%d postings — company already covered by a non-aggregator source"`.

## 3. Meili-backed coverage adapter

- [ ] 3.1 In `internal/search`, add a type implementing `pipeline.CoverageLookup`: given a batch of company slugs and the aggregator list, query Meili's `jobs` index with `filter: company_slug IN [batch] AND NOT source IN [aggregators]`, `facets: [company_slug]`, and return the facet distribution's keys as the covered set.
- [ ] 3.2 Batch the caller-supplied company slugs into chunks of ~500 before querying (the adapter owns batching, not the pipeline caller), and union the per-batch results into one `map[string]bool`.

## 4. Wiring

- [ ] 4.1 In `cmd/ingest/main.go`, construct the Meili adapter from 3.1 and set `Runner.Coverage` when `MEILI_URL` is configured; leave it nil otherwise (today's behavior — write everything, `cmd/reindex`'s `aggregator-ats-dedup` pass suppresses later).

## 5. Tests

- [ ] 5.1 Pipeline unit test: a fake `CoverageLookup` proving an aggregator posting for a covered company is skipped and counted in `Stats.ATSCovered`, not saved, not `Rejected`.
- [ ] 5.2 Pipeline unit test: an aggregator posting for an uncovered company is saved normally (`Stats.Ingested` increments).
- [ ] 5.3 Pipeline unit test: an ATS-provider (`KindATS`) board ignores a configured `Coverage` port entirely — every posting saves as it does today even when the fake would report the company covered.
- [ ] 5.4 Pipeline unit test: `Runner.Coverage == nil` reproduces today's behavior byte-for-byte for an aggregator board (no gate applied, no error).
- [ ] 5.5 Pipeline unit test: folded-slug matching — a covered-set entry differing from the posting's `company_slug` only by hyphenation still matches (mirrors the fold `aggregator-ats-dedup` already uses).
- [ ] 5.6 `internal/search` unit test for the adapter: verifies the filter/facet query shape and the batching behavior (chunking a company-slug list larger than one batch into multiple queries, unioning results).
- [ ] 5.7 `go vet -tags=integration ./...` — confirm no integration-tagged test in `internal/pipeline`/`cmd/ingest` breaks on the new `Runner` field or `saveOne` signature change.

## 6. Manual verification (post-deploy)

- [ ] 6.1 Add `MEILI_URL` to the aggregator-provider ingest systemd units (ops change, not code).
- [ ] 6.2 After deploy, confirm a Himalayas ingest run's log shows a nonzero `ATSCovered` count on at least one board, and that `Stats.Ingested` for that run is lower than the same board's pre-change baseline.
- [ ] 6.3 Spot-check on prod: pick a few companies from the pre-change 10,531-uncovered set (from the original measurement) and confirm their aggregator postings still ingest normally (the gate must not fire for genuinely uncovered companies).
