## Context

The aggregator suppression pass (`SuppressAggregatorDuplicatesForCompany`) currently matches an
aggregator posting to an ATS twin on two equality keys: `ntitle` (exact) and `ntitle2`
(entity-decoded + trailing-suffix-stripped), UNION ALL-ed as two hash joins. Both are *equality*
matches, so an aggregator that drops a word from the MIDDLE of the title (not a trailing suffix)
still misses. A spike measured the residual with a word-subset proxy: on the worst word-drop
syndicator (Aster DM Healthcare) exact+normalization catch 62/586 while word-containment adds ~234;
catalogue-wide the residual is ~1–2k. The identity-URL alternative was spiked and invalidated
(gulftalent/himalayas expose no raw ATS URL).

## Goals / Non-Goals

**Goals:**
- Catch aggregator postings whose title words are a subset of an ATS twin's, deterministically,
  with the built-in `<@` array operator (no `pg_trgm`, no threshold).
- Bound over-merge with a seniority/qualifier gate and a minimum token count.
- Keep every existing invariant and the same reindex wiring; additive third match arm.
- No new extension. No index unless proven necessary.

**Non-Goals:**
- Trigram/similarity matching (spike: over-built for this residual).
- Identity-URL mining for gulftalent/himalayas (spike: invalidated).
- Changing the exact/normalized paths or any `duplicate_of` consumer.

## Decisions

### Decision: A third `UNION ALL` arm — word-subset containment

Add to the `matches` CTE:

```
SELECT a.id, t.id
FROM agg a JOIN ats t
  ON string_to_array(a.ntitle,' ') <@ string_to_array(t.ntitle,' ')   -- agg words ⊆ ats words
 AND array_length(string_to_array(a.ntitle,' '),1) >= 2               -- not a 1-word generic
 AND <seniority gate>
 AND <country gate>
```

`MIN(ats_id)` over the union still picks a stable target; the LEFT JOIN back to `agg` still yields
NULL for unmatched rows (failover preserved). The exact/normalized arms are untouched, so this can
only *add* matches.

- **Alternative — trigram (`pg_trgm`) similarity:** rejected by spike. It needs an extension + GIN
  index + threshold tuning and carries higher over-merge risk, for a residual that word-subset
  (stricter, deterministic) already captures.
- **Alternative — replace equality with containment:** rejected. Containment is a superset of the
  exact match but changes proven behavior and can't hash-join; keeping the fast equality arms and
  adding containment as a third arm is strictly safer.

### Decision: Seniority gate — reject seniority-only differences

The over-merge risk is `Software Engineer` (agg) ⊆ `Senior Software Engineer` (ats). Gate: the match
is kept only when `(ats_tokens − agg_tokens)` contains at least one token that is NOT a
seniority/qualifier marker. So an ATS title that adds *only* a seniority word over the aggregator is
not merged (distinct grade), while one that adds a location/department/specialty word is (the
aggregator dropped it). The seniority marker set is a small curated SQL array (senior, sr, junior,
jr, lead, principal, staff, mid, entry, chief, head, intern, trainee, graduate, …); it mirrors the
`internal/classify` seniority vocabulary and can be sourced from it later.

- Residual over-merge beyond seniority (a bare title matching the wrong `Base + specialty` ATS row,
  e.g. `Software Engineer` ⊆ both `... Backend` and `... Frontend`) is accepted: aggregator-only,
  reversible, reachable-by-link, un-suppresses on twin close — the #610/#612 safety net.

### Decision: No index initially

The containment arm cannot hash-join (nested loop per company), but it runs only on the residual
(rows the equality arms did not catch) within a single company's rows, and the pass is per-company,
best-effort, log-and-continue. The spike ran full Marriott (2,990 × 2,815) in <45s, so the residual
is faster. A GIN functional index on `string_to_array(<ntitle expr>,' ')` (built-in array GIN, no
extension) is the perf lever if reindex wall-clock regresses — documented as a seam, not built now.

## Risks / Trade-offs

- **Over-merge on qualifier drops (non-seniority)** → Accepted, bounded by the safety net; the
  seniority gate covers the most common false case. If review wants it tighter, raise the minimum
  token count or require the shared token count to be a majority of the ATS title.
- **Containment arm cost on huge companies** → Bounded (<45s full-Marriott in the spike; residual is
  smaller) and best-effort; the GIN seam is the escape hatch.
- **Seniority set incompleteness** → Only lets a seniority-only pair slip through as a match (mild
  over-merge) or, if over-broad, blocks a real match (under-catch); start conservative.

## Migration Plan

Code-only, no migration. Activates on the next reindex. Rollback reverts the code; values it set
remain valid duplicates.

## Open Questions

- Exact seniority marker set (start from `internal/classify`) — confirm at implementation.
- Minimum aggregator token count (2 vs 3) — 2 with the seniority gate; tighten to 3 if review prefers.
