## 1. Sizing spike (feasibility of the impl, not the signal — signal already VALIDATED)

- [x] 1.1 Measure the bucket-size distribution — DONE 2026-07-30: 210 319 buckets, median 2, p99 27, largest 16 745, 165.4M naive pairs. Eleven buckets >1000 rows carry 92% of the cost, so a size cap (~200) replaces MinHash/LSH: 0.05% of buckets skipped, 97% of work removed, ~5.2M comparisons left. See design D6.
- [x] 1.2 Threshold validated on prod buckets (900 buckets, 3182 rows): T=0.85 merges 1360, T=0.90 merges 1293, T=0.95 merges 1156 — the curve is flat across the band the spike predicted, so 0.90 is not a knife edge. Sampled merges are real: same role in two cities whose descriptions differ by the city name alone (4 characters in 3767), which a hash can never see as "almost equal".
- [x] 1.3 Real collapse potential measured by dry run rather than by the misleading stripped-title count. On a deliberately candidate-heavy sample (buckets of 2-30 only) 1418 of 3182 rows merge at T=0.90; catalogue-wide the share is far lower, since the median bucket after the exact passes holds 2 rows and most singletons never enter.

- [x] 1.4 Measure the empty-`company_slug` hazard — DONE 2026-07-30: 154 900 rows (5.7%) carry none, 105 212 of them land in 20 126 cross-employer same-title buckets (up to 4 companies in one). Such rows are excluded from the pass; the bucket is only a guard when the slug is real. See design D7.

## 2. Similarity + query layer

- [x] 2.1 Implement the normalized-description word-signature (distinct lowercase tokens len>2) + within-bucket pairwise Jaccard as a pure, unit-tested function (no LSH — see D6).
- [x] 2.2 Add SQL to stream `(company_slug, normalized-title)` buckets of exact-pass leftover canons with their descriptions, skipping buckets above the size cap and rows with an empty `company_slug` (D6, D7).
- [x] 2.3 Reuse/extend the `duplicate_of` writer (idempotent `IS DISTINCT FROM`), mirroring the exact pass.

## 3. Dedup pass

- [x] 3.1 Per-bucket clustering (similarity >= T -> same cluster, `min(id)` canon), unit-tested. The seniority-grade guard turned out unnecessary — the bucket key is the whole normalized title, so grades are already separate buckets (design D5).
- [x] 3.2 Wire into `cmd/reindex` AFTER `recomputeRoleDuplicates`/`suppressAggregatorDuplicates` (or a dedicated worker), over leftover canons only.
- [x] 3.3 Unit tests: near-identical merges; distinct-job (amazon-style) stays split; mixed-specialty (speechify-style) stays split; grade guard; idempotent re-run.

## 4. Verification

- [x] 4.1 Dry run over real prod buckets inspected for false positives: the sampled merges are per-city variants of one role (Growth Manager Berlin/Stockholm, Investment Manager Berlin/Stockholm) and one cross-source pair (teamtailor + jobtech) whose descriptions differ by 400 characters but share their vocabulary. No merge of unrelated roles observed at 0.90.
- [ ] 4.2 Confirm the geo-union (`ingest-content-dedup`) still widens the fuzzy-merged canons; confirm Towa Kraków/Wien now fold into the fullstack canon.
- [x] 4.3 `go build ./... && go vet ./... && go test ./...` green.
