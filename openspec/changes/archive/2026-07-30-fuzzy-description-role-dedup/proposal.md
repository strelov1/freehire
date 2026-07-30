## Why

The role-cluster collapse (`ingest-content-dedup`) keys on an EXACT description match as
its over-merge guard, so a role reposted per-city with a slightly localized description
(a country-specific salary/legal block) does not collapse — e.g. Towa's "Senior Fullstack
Engineer": Bregenz/Düsseldorf/München (byte-identical) collapsed, but Kraków and Wien
(~120–200 chars different out of ~11,000, >98% identical) stayed separate cards.

The first follow-up tried **embedding cosine** to bridge this and was **INVALIDATED by a
spike** (see the shelved `semantic-role-dedup`): boilerplate-dominated embeddings cannot
separate same-role city variants from distinct roles.

A second spike VALIDATED a better signal — **word-Jaccard of the normalized description**:

| Bucket | same-role city variants | distinct roles in same bucket |
|---|---|---|
| Towa `senior fullstack engineer` | 0.976–1.000 | 0.48 (Data Specialist / Consultant) |
| speechify `software engineer` | 0.954 (DataInfra) | 0.44–0.46 (Platform / iOS) |
| amazon `software development engineer` | — | avg 0.186 (261 genuinely-distinct jobs) |

The gap is wide and consistent (true dupes ≥0.95, distinct roles ≤0.5) exactly where the
embedding failed. Crucially, amazon's 228-card "SDE" bucket correctly does NOT collapse
(avg 0.186) — those are different jobs under a generic title, not dupes.

## What Changes

- Extend the role-cluster collapse with a **fuzzy-description** pass: within a
  `(company_slug, stripped-title)` bucket, cluster open canonical postings whose normalized
  descriptions exceed a word-similarity threshold and mark all but one `duplicate_of` the
  canon — the same collapse mechanism, an exact-match guard relaxed to near-match.
- Runs AFTER the exact role-cluster recompute, over its leftover canons only, strictly
  additive: it merges what byte-exact matching left split, never re-splits a deterministic
  collapse.
- Guards: the shared stripped-title bucket bounds comparison and prevents cross-role merges;
  a conservative threshold (spike shows ≥0.9 sits well inside the true-dupe band and far
  above any distinct-role pair); the existing seniority-grade guard.

## Capabilities

### New Capabilities
- `fuzzy-description-role-dedup`: collapse near-identical-description reposts that exact
  matching misses, using a normalized-description word-similarity within a company+title bucket.

### Modified Capabilities
<!-- none: reuses ingest-content-dedup's duplicate_of column, reindex exclusion, /copies, and geo-union unchanged. -->

## Impact

- **Code:** a fuzzy pass (in `cmd/reindex` after `recomputeRoleDuplicates`, or a dedicated
  worker), a description word-signature + within-bucket similarity, writing `duplicate_of`.
- **Efficiency:** pairwise word-Jaccard is O(n²) per bucket; buckets are bounded per company,
  but large ones (amazon 261, speechify 305) need an efficient signature (MinHash/LSH, a
  shingle simhash, or `pg_trgm` GIN) rather than naïve pairwise — a design decision.
- **Data:** more `duplicate_of` rows; reindex/copies/geo-union machinery unchanged; reversible.
- **Risk:** over-merge of near-but-distinct roles — mitigated by bucket + conservative
  threshold + grade guard; measure the false-positive rate on a prod sample before enabling.
- **Supersedes** the shelved `semantic-role-dedup` (embedding approach, INVALIDATED).
