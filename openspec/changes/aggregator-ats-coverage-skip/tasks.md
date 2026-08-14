## 1. Pipeline port and stats

- [x] 1.1 In `internal/pipeline/pipeline.go`, add the `CoverageLookup` interface (`NonAggregatorCompanies(ctx, companySlugs, aggregators []string) (map[string]bool, error)`) next to `BoardHealth`, and a new `Runner.Coverage CoverageLookup` field.
- [x] 1.2 Add `Stats.ATSCovered int` and the matching `s.ATSCovered += o.ATSCovered` line in `Stats.add`.

## 2. Ingest-time gate

- [x] 2.1 Define the `aggregatorCoverage func(companySlug string) bool` resolver type in `internal/pipeline/pipeline.go`. Change `saveOne`'s signature to accept one (nil-able), and — after the existing `outOfCatalogue` check, before `r.save` — skip the write when `covered != nil && covered(dj.Fields().CompanySlug)`, incrementing `Stats.ATSCovered` and returning, without touching `rej`/`Rejected`.
- [x] 2.2 In `ingestFetched` (buffered path), once per board before the per-posting loop: when `r.Coverage != nil` and `sources.ProviderKind(sources.Taxonomy(), e.Provider) == sources.KindAggregator`, collect the distinct `company_slug` values from `raw`, resolve them via one batched `r.Coverage.NonAggregatorCompanies` call, and wrap the result map in an `aggregatorCoverage` closure. Pass `nil` for every non-aggregator board or when `r.Coverage == nil`.
- [x] 2.3 In `ingestStream` (streaming path, e.g. `jobtech`), under the same gate condition as 2.2: build an `aggregatorCoverage` closure that calls `r.Coverage.NonAggregatorCompanies` with a single-element slice per distinct company, memoized in a map local to the stream's `emit` closure (guarded by the mutex `emit` already holds) so a company with several postings in one stream is looked up once.
- [x] 2.4 Add the per-board log line (mirroring `rejections.log`), emitted only when `ATSCovered > 0`, in both `ingestFetched` and `ingestStream`'s summary logging: `"ingest: %s board %q (%s): skipped %d/%d postings — company already covered by a non-aggregator source"`.
- [x] 2.5 (found in review) `ingestBoard`'s streaming "no progress" board-health check must also treat an all-`ATSCovered` prefix as progress, the same way it already treats an all-`Rejected` prefix — otherwise a heavily-covered streaming aggregator board that later fails gets misclassified as a true outage and cooled down.
- [x] 2.6 (found while implementing task 3.1) **Remove folding.** `distinctFoldedCompanySlugs`, `foldCompanySlug`, and the fold step in both `aggregatorCoverageForBatch` and `streamAggregatorCoverage`'s closures must go: a live Meili filter cannot compute the reindex pass's `replace(company_slug, '-', '')` fold at query time, and folding the query value before sending it to Meili breaks even the ordinary, correctly-spelled case (it no longer matches the literal stored value). `CoverageLookup`'s doc comment must drop the "companySlugs are already FOLDED" claim — plain, unfolded `company_slug` (`normalize.Slug` output, as-is) flows over the interface on both the request and the returned map's keys. See design.md's "Coverage definition" (NO folding) and the new Risk entry.

## 3. Meili-backed coverage adapter

- [x] 3.1 In `internal/search`, add a type implementing `pipeline.CoverageLookup`: given a batch of company slugs and the aggregator list, query Meili's `jobs` index with `filter: company_slug IN [batch] AND NOT source IN [aggregators]`, `facets: [company_slug]`, and return the facet distribution's keys as the covered set. (Implemented as a method directly on `*search.Client` rather than a separate wrapper type — Go's structural interfaces mean `internal/search` needs no import of `internal/pipeline` at all, and a wrapper type would add nothing `Client` doesn't already provide.)
- [x] 3.2 Batch the caller-supplied company slugs into chunks of ~500 before querying (the adapter owns batching, not the pipeline caller), and union the per-batch results into one `map[string]bool`.

## 4. Wiring

- [x] 4.1 In `cmd/ingest/main.go`, construct the Meili adapter from 3.1 and set `Runner.Coverage` when Meili is configured; leave it nil otherwise (today's behavior — write everything, `cmd/reindex`'s `aggregator-ats-dedup` pass suppresses later). Gated on `cfg.MeiliKey != ""` (the established "MeiliKey empty ⇒ search disabled" convention `cmd/server` already uses), not literally `MEILI_URL` — `MeiliURL` always has a default value, so it can never be used as the "is search configured" signal.

## 5. Tests

- [x] 5.1 Pipeline unit test: a fake `CoverageLookup` proving an aggregator posting for a covered company is skipped and counted in `Stats.ATSCovered`, not saved, not `Rejected`.
- [x] 5.2 Pipeline unit test: an aggregator posting for an uncovered company is saved normally (`Stats.Ingested` increments).
- [x] 5.3 Pipeline unit test: an ATS-provider (`KindATS`) board ignores a configured `Coverage` port entirely — every posting saves as it does today even when the fake would report the company covered.
- [x] 5.4 Pipeline unit test: `Runner.Coverage == nil` reproduces today's behavior byte-for-byte for an aggregator board (no gate applied, no error).
- [x] 5.5 (revised — folding removed per task 2.6) Pipeline unit test: EXACT `company_slug` matching only — a covered-set entry differing from the posting's `company_slug` only by hyphenation (e.g. `cfoinsights` vs `cfo-insights`) does NOT match, proving the pipeline no longer folds before comparing (replaces the old folded-match assertion, which is no longer true).
- [x] 5.6 Pipeline unit test: a streaming aggregator source (`jobtech`) with a fake `CoverageLookup` — a covered company's postings are skipped via `ingestStream`'s per-company resolver, and the fake is called once per distinct company (not once per posting) across multiple postings for the same company in one stream.
- [x] 5.6a (found in review) Pipeline unit test: an all-`ATSCovered` prefix on a streaming board that then fails is recorded as a board success, not a failure — the `Stats.ATSCovered` sibling of the existing all-`Rejected` case.
- [x] 5.6b (found in review) Pipeline unit test: the `aggregators` argument `NonAggregatorCompanies` receives is the real, non-empty `sources.AggregatorProviders(sources.Taxonomy())` list (closes a gap where every existing test discarded this argument).
- [x] 5.6c (found in review) Pipeline unit test: a streaming posting with a blank company never reaches `CoverageLookup` (nothing meaningful to ask Meili about).
- [x] 5.7 `internal/search` unit test for the adapter: verifies the filter/facet query shape and the batching behavior (chunking a company-slug list larger than one batch into multiple queries, unioning results).
- [x] 5.8 `go vet -tags=integration ./...` — confirm no integration-tagged test in `internal/pipeline`/`cmd/ingest` breaks on the new `Runner` field or `saveOne` signature change.

## 6. Manual verification (post-deploy)

- [ ] 6.1 Confirm whether `MEILI_MASTER_KEY` already reaches the aggregator-provider ingest systemd units' environment (four other workers already require it — it may already be fleet-wide) and add it if not (ops change, not code). If it's already fleet-wide, this gate activates for every aggregator provider on the very next deploy, not gradually — confirm that's the intended rollout, not a surprise.
- [ ] 6.2 After deploy, confirm a Himalayas ingest run's log shows a nonzero `ATSCovered` count on at least one board, and that `Stats.Ingested` for that run is lower than the same board's pre-change baseline.
- [ ] 6.3 Spot-check on prod: pick a few companies from the pre-change 10,531-uncovered set (from the original measurement) and confirm their aggregator postings still ingest normally (the gate must not fire for genuinely uncovered companies).
