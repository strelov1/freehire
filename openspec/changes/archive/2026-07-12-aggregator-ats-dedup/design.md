## Context

Freehire already has a role-cluster repost dedup (`ingest-content-dedup`): per company,
open postings sharing a `role_fingerprint` (`company_slug` + normalized title + normalized
description) collapse to one canonical row via the `duplicate_of` column, and every active
surface — job search / Meilisearch, semantic embedding, LLM enrichment, list / sitemap /
company reads — filters `duplicate_of IS NULL`. Detail-by-slug and the
`job-cluster-copies` endpoint deliberately still serve duplicates.

That machinery is exactly what we want for aggregator suppression, but its **membership
key is wrong for the cross-source case**. Because `role_fingerprint` includes the
description, and aggregators rewrite/​truncate descriptions, an aggregator posting and its
first-party ATS twin fingerprint differently and never share a cluster. On prod only ~115
mixed clusters exist, yet ~9,442 open aggregator postings have an exact-title ATS twin in
the same company and compatible country. Those 9,442 are indexed, embedded, and enriched
as distinct jobs today.

## Goals / Non-Goals

**Goals:**
- Suppress an open aggregator posting when an open first-party ATS posting of the same
  company, title, and compatible country exists, keeping the ATS copy canonical.
- Reuse the existing `duplicate_of` column and every surface that already filters it — no
  changes to search, embedding, enrichment, detail, or cluster-copies.
- Safe by construction: never demote an ATS row, never merge across countries, never
  merge aggregator↔aggregator.
- No schema change, no migration.

**Non-Goals:**
- Fuzzy / partial title matching for aggregators that mangle titles (gulftalent's Marriott
  copies). That is a separate, riskier follow-up.
- Changing the role-cluster repost dedup or its `role_fingerprint`.
- Skipping ingest of the aggregator posting entirely (we store it; it stays reachable by
  link and as a cluster copy).

## Decisions

### Decision: A separate suppression pass, not a change to `role_fingerprint`

Add a new per-company query `SuppressAggregatorDuplicatesForCompany` (with a
`CompaniesWithAggregatorPostings` driver), run in the reindex reconcile right after
`recomputeRoleDuplicates` — the reindex is where `duplicate_of` markers are already
refreshed and the search index is rebuilt from them. It sets `duplicate_of` on an open
aggregator row to the id of an open canonical ATS row (`duplicate_of IS NULL`) of the same
`company_slug`, equal normalized title, and compatible country.

- **Alternative — loosen `role_fingerprint` to drop the description:** rejected. The
  description is what keeps the repost pass from over-merging genuinely distinct roles
  within a source; dropping it would regress repost behavior. The two passes have
  different safety needs and stay separate.
- **Alternative — precedence flip (`MIN(id)` → ATS-first) in the existing pass:**
  rejected as insufficient. It would only reorder the ~46 already-mixed clusters; the
  9,442 real duplicates never cluster in the first place, so a canon reorder does nothing
  for them.

### Decision: Normalized title equality, exact (no suffix strip, no entity decode)

Reuse the role pass's title normalization (lowercase, non-alphanumeric runs → single
space). No suffix stripping after `-`/`|` and no HTML-entity decoding in this slice — that
edges into fuzzy matching and belongs to the follow-up. Exact-title already covers the
clean aggregators (himalayas ~65% exact title match, the Danish portals) and the
exact-matching subset of gulftalent.

### Decision: Country compatibility gate = array overlap OR either side empty

`a.countries && b.countries OR cardinality(a.countries)=0 OR cardinality(b.countries)=0`.
This blocks the 7-Eleven false-merge (Singapore aggregator posting vs US ATS posting of
the same title) while still matching when geography is unresolved on one side (the
`countries` dictionary is deliberately sparse and "never guesses", so an empty array must
not veto a real match). Country, not city/region, is the right granularity: an aggregator
and an ATS may phrase the city differently but agree on the country.

### Decision: Aggregator set sourced from the Go registry, passed as a query param

Add `sources.AggregatorProviders(reg) []string`, derived from the existing `aggregator()`
interface markers (single source of truth). The reindex caller passes it into the query as
a `text[]` param. No persisted `source_is_aggregator` column, so no migration and no
backfill; the set updates automatically when a source's marker changes.

### Decision: Idempotent, failover-safe re-evaluation

Follow the existing recompute's shape: compute the desired `duplicate_of` for each
candidate aggregator row and write only where it `IS DISTINCT FROM` the current value. A
suppressed row whose ATS twin has closed finds no open canonical ATS match and is set back
to NULL, re-entering the active surfaces. Ordering matters: run the role-cluster recompute
first (so ATS reposts collapse to their own canonical row), then this pass (so aggregator
rows point at the ATS *canonical* row, never at an ATS duplicate).

Candidate rows are open aggregator rows that are **canonical OR already point at a
non-aggregator row** (i.e. suppressed by this pass). An aggregator row the role pass
pointed at *another aggregator* (a cross-aggregator repost) is excluded, so this pass never
clobbers the role pass's decision — the two passes own disjoint sets of `duplicate_of`
edges.

## Risks / Trade-offs

- **Over-suppression from a coincidental same title in the same company+country** (e.g.
  two genuinely distinct "Software Engineer" openings, one on the ATS, one on the
  aggregator) → Mitigated but not eliminated by the exact-title + country gate. Acceptable
  for this slice: the aggregator copy is only hidden from search/embedding/enrichment, is
  never deleted, is still reachable by link and as a cluster copy, and un-suppresses if the
  ATS twin closes. This mirrors the risk profile the role-cluster pass already accepts.
- **Aggregator with mangled titles is missed** (gulftalent's suffix-stripped Marriott
  copies) → Out of scope by design; the exact-title key simply does not match them, so
  they stay independent — no regression, just not-yet-caught. Follow-up fuzzy slice.
- **Cross-pass interaction** → The role-cluster pass and this pass both write
  `duplicate_of`. Running role-cluster first and pointing only at canonical (`duplicate_of
  IS NULL`) ATS rows keeps them consistent; both use the `IS DISTINCT FROM` guard so their
  combined re-run stays idempotent.

## Migration Plan

No schema migration. Ships as code only. The suppression runs inside the reindex, so the
first reindex after deploy suppresses the matching aggregator copies and rebuilds the
index without them; they drop out of embedding/enrichment on the next worker pass (whose
enqueue already filters `duplicate_of`). Rollback is reverting the code — the values this
pass set persist harmlessly until a run touches them, and they remain valid duplicates
regardless.

## Open Questions

- None blocking. The fuzzy-title follow-up (gulftalent) is tracked as a separate change,
  not an open question here.
