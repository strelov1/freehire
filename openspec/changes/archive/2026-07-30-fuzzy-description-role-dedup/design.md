## Spike verdict (2026-07-20): ✅ VALIDATED

Read-only prod spike, word-Jaccard of the normalized description (distinct lowercase tokens
of length >2, `|A∩B|/|A∪B|`):

| Bucket | same-role city variants | distinct roles |
|---|---|---|
| Towa `senior fullstack engineer` | 0.976–1.000 | Data Specialist 0.49, Management Consultant 0.48 |
| speechify `software engineer` | DataInfra 0.954 | Platform 0.44, iOS 0.46 |
| amazon `software development engineer` | (1 near-dupe) | 261 rows, avg 0.186 |

Wide, consistent gap (true dupes ≥0.95, distinct ≤0.5) — and it works EXACTLY where the
embedding approach failed (speechify 0.968 vs 0.966; amazon 0.83–0.97). Amazon's generic-title
bucket correctly does not collapse (avg 0.186 = genuinely distinct jobs). Any threshold in
0.6–0.9 separates cleanly; ≥0.9 is conservative and safe.

## Context

`ingest-content-dedup` collapses on EXACT description match. Near-identical-but-localized
descriptions (Towa Kraków/Wien, >98% identical) stay split. The prior embedding follow-up was
INVALIDATED (`semantic-role-dedup`, shelved). Word-Jaccard of the description is the validated
signal.

## Goals / Non-Goals

**Goals:**
- Collapse near-identical-description reposts that exact matching misses.
- Zero regression to the deterministic exact pass; strictly additive.
- Bounded cost; reversible; measurable before enabling.

**Non-Goals:**
- Merging genuinely distinct jobs under a generic title (amazon SDE) — the threshold must
  leave those split.
- Cross-company/cross-bucket merging.
- The `company_slug` duplication issue (`jp-morgan-chase`/`jpmorganchase`) — separate.

## Decisions

### D1 — Signal: word-Jaccard of the normalized description (not embeddings)
Spike-proven to separate dupes from distinct roles where embeddings could not (boilerplate
dominates embeddings; word-overlap captures the specialty-specific body). This is the exact-md5
guard relaxed to ≥T word overlap.

### D2 — Bucket by (company_slug, stripped-title); cluster within
Same bucketing as the exact pass bounds comparison and prevents cross-role merges. Within the
bucket, cluster by Jaccard ≥ threshold, `min(id)` canon.

### D3 — After the exact pass, additive only
Reads only exact-pass leftover canons (`duplicate_of IS NULL`), only adds markers. Reuses the
column, reindex exclusion, `/copies`, geo-union unchanged.

### D4 — Efficient signature, not naïve O(n²)
Pairwise Jaccard per bucket is O(n²); large buckets (amazon 261, speechify 305) need a
signature: MinHash/LSH banding, a shingle simhash, or a `pg_trgm` GIN with `similarity()`.
Pick during implementation after the spike sizes bucket distribution. Conservative threshold
≥0.9 (well inside the ≥0.95 true-dupe band, far above ≤0.5 distinct).

### D5 — Grade guard — NOT NEEDED, the bucket already is one

Superseded during implementation. The bucket key is the whole normalized title, and a grade word is
part of the title, so `senior software engineer` and `software engineer` are different buckets before
any description is read. The explicit guard the SQL subset arm needs exists because that arm compares
ACROSS titles; this pass never does. Adding it here would be a check that can never fire.

## Risks / Trade-offs

- **Over-merge of near-but-distinct roles** → bucket + conservative threshold + grade guard;
  measure false-positive rate on a prod sample before enabling. Spike margin is large.
- **O(n²) cost on big buckets** → signature/LSH; only buckets with >1 canon; existing reindex
  cadence.
- **Threshold drift** → single conservative global threshold first; the spike shows the gap is
  company-independent so far.

## Sizing spike (2026-07-30) — answers D4 and adds a guard the design was missing

Measured over open canonical rows on prod, bucketing by `(company_slug, normalized title)`:

```
buckets with >1 canon    210 319        median bucket size     2
rows in them             851 920        p99                   27
largest bucket            16 745        naive pairwise    165.4M
```

| bucket size | buckets | rows | pairwise comparisons |
|---|---|---|---|
| 2–10 | 201 211 | 572 598 | 772 221 |
| 11–50 | 8 345 | 156 209 | 1 685 087 |
| 51–200 | 685 | 57 596 | 2 777 596 |
| 201–1000 | 87 | 33 935 | 8 294 788 |
| >1000 | **11** | 31 661 | **151 889 885** |

**D6 — Cap the bucket size instead of reaching for MinHash/LSH.** Eleven buckets carry 92% of the
cost. Skipping buckets above ~200 rows drops 0.05% of buckets and 97% of the work, leaving ~5.2M
comparisons over 787k rows — naive pairwise per bucket, no LSH, no `pg_trgm`, no new index.

The cap is not only a cost trade: a bucket that large is generic-title-by-location, which the design
already says must NOT collapse. The largest are `dollar-tree` "customer service associate i" (16 745),
"assistant manager i" (1 843), "part time sales teammate" (1 780) — one role across thousands of
stores. Skipping them is the same verdict the threshold would reach, reached for free.

**D7 — Empty `company_slug` breaks the bucket guard; exclude it.** 154 900 open canonical rows
(5.7%) have no `company_slug`, and 105 212 of them fall into 20 126 same-title buckets — grouped
ACROSS employers, with up to 4 distinct companies in one bucket. The design treats the bucket as the
thing that "prevents cross-role merges", but with an empty slug it stops being a company boundary at
all. Rows without a `company_slug` are therefore excluded from the pass entirely: they are exactly
where a boilerplate-driven high similarity would merge two different employers' jobs.

## Open Questions

- Exact threshold (0.85 vs 0.9 vs 0.95) — the original spike put true dupes ≥0.95 and distinct roles
  ≤0.5, so anything in the gap works; ≥0.9 stays the candidate, to be confirmed on the labelled
  sample during implementation.
- How many cards this actually collapses — a real count (within-bucket ≥T pairs), NOT the
  misleading 849k stripped-title figure (which counted distinct jobs too).
