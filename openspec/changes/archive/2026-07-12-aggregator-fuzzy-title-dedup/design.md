## Context

`aggregator-ats-dedup` (shipped) suppresses an aggregator posting when its normalized title
— `btrim(regexp_replace(lower(title),'[^a-z0-9]+',' ','g'))` — exactly equals an open
canonical ATS posting's, in the same company and a compatible country. The normalization is
strict: it lowercases and collapses non-alphanumeric runs, but does nothing about HTML
entities or appended suffixes.

Per-aggregator prod measurement of the exact catch:

| aggregator | open | exact ATS twin |
|---|---|---|
| himalayas | 15,255 | 4,563 |
| gulftalent | 23,667 | 3,256 |
| workatastartup | 4,969 | 935 |
| jobstash | 3,489 | 759 |
| justjoin | 8,947 | 375 |
| others | — | <200 each |

gulftalent is the standout under-catch: its high-volume employers (Marriott, Apparel Group,
Aster) syndicate from Oracle/Taleo with the title mangled, so exact misses them. himalayas
preserves titles (~65% exact on Instacart), so its residual fuzzy upside is small; the
national portals are largely exclusive. A naive prefix/`LIKE` cross-join to catch the
mangled cases timed out at 120s on Marriott alone (2,990 × 2,815, no index on the computed
title) — confirming a blanket similarity join is the wrong tool here.

## Goals / Non-Goals

**Goals:**
- Catch the two deterministic title-mangling classes — undecoded HTML entities and an
  appended ` - <suffix>` — with a better normalization key, as an additive OR path beside
  the exact key.
- Keep every `aggregator-ats-dedup` invariant (aggregator-only, ATS never demoted, country
  gate, idempotent failover, reachable-by-slug), the same reindex wiring, and the same
  O(agg+ats) hash-join cost.
- No schema change, no Postgres extension.

**Non-Goals:**
- Fuzzy/similarity matching for reworded or partially-dropped titles (`Waiter` vs
  `Waiter/ Waitress`) — a separate trigram (`pg_trgm`) follow-up.
- Changing the exact key or any downstream `duplicate_of` consumer.
- Touching himalayas/national-portal behavior (already well-covered or exclusive).

## Decisions

### Decision: A normalized key as an additional OR match path, not a replacement

Compute a second key `ntitle2 = collapse(strip_suffix(decode_entities(title)))` on BOTH the
aggregator and the ATS side, and suppress when the aggregator matches an ATS twin on
`ntitle = ntitle` (existing) **OR** `ntitle2 = ntitle2`. Additive, so slice-1's behavior is
untouched and a regression is impossible for exact matches.

- **Alternative — replace the exact key with ntitle2:** rejected. It would change slice-1's
  proven behavior and risk new mismatches; additive is strictly safer.
- **Alternative — trigram similarity now:** rejected for this slice. It needs `pg_trgm` +
  a GIN index + threshold tuning, carries higher over-merge risk, and the residual upside
  after normalization is unmeasured. Deferred to its own change.

### Decision: Entity-decode before the alphanumeric collapse

`&amp;`/`&#38;`/`&amp;amp;` → `&` (and the other common named/numeric entities) applied to
the raw title first, so `F&amp;B` and `F&B` both collapse to `f b`. This is a lossless,
deterministic character substitution — **no over-merge risk** (it only makes two spellings
of the same string agree). Implemented as a small fixed set of `replace()` calls in SQL (or
a helper) — the entity set seen in ATS/aggregator titles is tiny.

### Decision: Strip one trailing separator suffix, conservatively

Remove a single trailing ` - …` / ` | …` / ` — …` segment before collapsing, so ATS
`Assistant Director of Sales - Leisure` matches aggregator `Assistant Director of Sales`.
Strip only the LAST separator segment (not every hyphen), and only when a non-empty base
remains, to avoid shredding hyphenated role names (`Full-Stack Engineer` has no
` - ` with surrounding spaces, so it is untouched — the space-delimited separator is the
guard).

## Risks / Trade-offs

- **Over-merge: a bare aggregator title matches the wrong `Base - Suffix` ATS row** (e.g.
  aggregator `Engineer` matched against ATS `Engineer - Frontend` when `Engineer - Backend`
  also exists) → Real but bounded exactly as slice-1: only an aggregator copy is hidden,
  never deleted, still reachable by link and as a cluster copy, and un-suppressed if the
  chosen ATS twin closes. The company+country gate limits blast radius. Net harm of a wrong
  pick between two near-identical roles of the same company is low. If review wants it
  tighter, gate the separator-strip on the suffix being short (≤ N words) or
  location-like — noted as a tunable.
- **Entity-decode covering an incomplete entity set** → Only reduces catch (a missed entity
  stays exact-only), never a wrong match. Start with the handful seen in real titles.
- **Separator-strip cost** → Still an equality hash-join on the precomputed `ntitle2`
  (O(agg+ats)); no `LIKE`/cross-join, so the Marriott timeout does not recur.

## Migration Plan

Code-only, same as slice-1. Activates on the next reindex; newly-matched aggregator copies
drop out of the rebuilt index and out of embedding/enrichment on the next worker pass.
Rollback is reverting the code; values it set remain valid duplicates.

## Open Questions

- Exact entity set to decode (start: `&amp; &#38; &quot; &#39; &apos; &lt; &gt;`) — confirm
  at implementation from a prod title sample.
- Whether to bound the separator-strip (suffix length/shape) for extra over-merge safety —
  default is strip-last-segment; tighten only if review prefers.
