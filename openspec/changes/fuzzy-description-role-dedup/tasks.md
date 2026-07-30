## 1. Sizing spike (feasibility of the impl, not the signal — signal already VALIDATED)

- [x] 1.1 Measure the bucket-size distribution — DONE 2026-07-30: 210 319 buckets, median 2, p99 27, largest 16 745, 165.4M naive pairs. Eleven buckets >1000 rows carry 92% of the cost, so a size cap (~200) replaces MinHash/LSH: 0.05% of buckets skipped, 97% of work removed, ~5.2M comparisons left. See design D6.
- [ ] 1.2 Pick and validate the threshold on a labelled prod sample (Towa, speechify, amazon + a random draw): confirm recall on true dupes and near-zero false positives at the chosen T (≥0.9 candidate).
- [ ] 1.3 Count the REAL collapse potential (within-bucket pairs/clusters ≥ T) — the honest figure, not the 849k stripped-title count.

- [x] 1.4 Measure the empty-`company_slug` hazard — DONE 2026-07-30: 154 900 rows (5.7%) carry none, 105 212 of them land in 20 126 cross-employer same-title buckets (up to 4 companies in one). Such rows are excluded from the pass; the bucket is only a guard when the slug is real. See design D7.

## 2. Similarity + query layer

- [x] 2.1 Implement the normalized-description word-signature (distinct lowercase tokens len>2) + within-bucket pairwise Jaccard as a pure, unit-tested function (no LSH — see D6).
- [ ] 2.2 Add SQL to stream `(company_slug, normalized-title)` buckets of exact-pass leftover canons with their descriptions, skipping buckets above the size cap and rows with an empty `company_slug` (D6, D7).
- [ ] 2.3 Reuse/extend the `duplicate_of` writer (idempotent `IS DISTINCT FROM`), mirroring the exact pass.

## 3. Dedup pass

- [ ] 3.1 Per-bucket clustering (similarity ≥ T → same cluster, `min(id)` canon) with the seniority-grade guard, unit-tested.
- [ ] 3.2 Wire into `cmd/reindex` AFTER `recomputeRoleDuplicates`/`suppressAggregatorDuplicates` (or a dedicated worker), over leftover canons only.
- [ ] 3.3 Unit tests: near-identical merges; distinct-job (amazon-style) stays split; mixed-specialty (speechify-style) stays split; grade guard; idempotent re-run.

## 4. Verification

- [ ] 4.1 On a prod copy (or read-only dry-run that logs would-merge sets), sample merges for false positives at the chosen T.
- [ ] 4.2 Confirm the geo-union (`ingest-content-dedup`) still widens the fuzzy-merged canons; confirm Towa Kraków/Wien now fold into the fullstack canon.
- [ ] 4.3 `go build ./... && go vet ./... && go test ./...` green.
