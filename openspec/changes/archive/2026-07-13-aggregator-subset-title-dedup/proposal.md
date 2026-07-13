## Why

`aggregator-ats-dedup` (#610) matches on an exact normalized title; `aggregator-fuzzy-title-dedup`
(#612) added an entity-decoded, suffix-stripped key. Together they catch ~70% of the worst
title-mangling syndicator (gulftalent/Marriott). The residual is aggregators that **drop words
from the middle** of the title, not just a trailing suffix — e.g. Aster DM Healthcare, where
exact+normalization catch only 62 of 586 postings but a word-containment match would catch ~234
more. A spike measured ~1–2k such residual duplicates catalogue-wide, concentrated in these
"word-drop" syndicators.

The precise alternative — mining the raw ATS apply URL and deduping by identity (the
`sources.NamespaceExternalID` / linksource pattern used for other adapters) — was spiked and
**invalidated for these aggregators**: gulftalent applies through itself, and himalayas exposes
no raw ATS URL (its API lacks one; its detail pages are Cloudflare-gated). So a title signal is
the only lever for these walled aggregators, and word-containment is the deterministic form of it.

## What Changes

- Add a third match path to `SuppressAggregatorDuplicatesForCompany`: an open **aggregator**
  posting is suppressed as `duplicate_of` an open canonical **ATS** posting of the same company
  and compatible country when the aggregator title's word set is a **subset** of the ATS title's
  word set (the aggregator dropped words the ATS keeps). Uses the built-in array containment
  operator (`<@`) — **no `pg_trgm`, no similarity threshold**.
- **Seniority gate** to bound over-merge: the match is rejected when the words the ATS title adds
  over the aggregator title are *only* seniority/qualifier markers (so `Software Engineer` is not
  merged into `Senior Software Engineer`). A minimum aggregator token count further guards short,
  generic titles.
- Additive: exact (#610) and normalized (#612) paths are unchanged; this is a third `UNION ALL`
  arm. Every existing invariant holds (aggregator-only, ATS never demoted, country gate,
  idempotent failover, reachable-by-slug, `duplicate_of` reuse).

Scope is **deterministic word-subset containment**. Trigram/similarity matching is explicitly out
(the spike showed it is over-built vs word-subset for this residual).

## Capabilities

### New Capabilities
- `aggregator-subset-title-dedup`: suppress an aggregator posting whose title words are a subset of
  a first-party ATS posting's, gated against seniority-only differences.

### Modified Capabilities
<!-- none — extends the aggregator suppression pass additively. -->

## Impact

- `internal/db/queries/jobs.sql` — third match arm (word-subset `<@` + seniority gate) in
  `SuppressAggregatorDuplicatesForCompany`; regenerate sqlc. Driver and `cmd/reindex` wiring
  unchanged.
- No `pg_trgm`, no new column. **No index initially** — the containment arm runs on the residual
  (rows not caught by the exact/normalized paths) per company, best-effort; a GIN functional index
  on the title-token array is a documented perf seam if reindex time suffers.
- **Over-merge risk** rises beyond the seniority gate (a bare title can match the wrong
  `Base + qualifier` ATS row); bounded by the same safety net as #610/#612 and analyzed in design.md.
