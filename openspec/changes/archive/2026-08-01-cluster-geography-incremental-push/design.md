## Context

`MergeClusterGeography` (`internal/search/document.go:84`) already does the widening and is
already tested. The gap is purely that three of its four potential callers have no way to ask
for one cluster's geography: the schema carries only `RoleClusterGeoAll`, a whole-catalogue
aggregate built for the reindex's one-pass lookup map.

The counts have both shapes already — `RoleClusterCountsAll` for the reindex and
`RoleClusterCount` for the per-row callers — so the missing query is the obvious sibling, not a
new concept.

Why the omission is destructive rather than merely incomplete: `SubmitJobs`
(`internal/search/client.go:496`) uses `UpdateDocumentsWithContext`, which merges by *field*.
`jobview.Job` declares `Countries`, `Regions` and `Cities` without `omitempty`, so those keys
are always in the payload and always replace whatever the reindex wrote.

## Goals / Non-Goals

**Goals:**

- An incremental push widens the canon exactly as the reindex does.
- No extra query for the common case (a role with one open posting).
- The requirement stops blessing the narrow behaviour.

**Non-Goals:**

- **Collapsing the four assembly preambles into one constructor.** All four repeat
  `repost, mass := 1, 1` → `RoleClusterCount` → `FromJob` → `ClassifyReality`, and after this
  change three of them repeat the geography step too. It is a real duplication and a reasonable
  follow-on, but it is a different change: `cmd/embed` deliberately skips reality in `pgOnly`
  mode, and the reindex feeds from prebuilt whole-catalogue maps rather than per-row queries, so
  the shared thing would need to take plain values rather than lookups. Doing it here would mix
  a behaviour fix with a refactor across four packages.
- Adding `omitempty` to the three facets. It would make an incremental push non-destructive by
  accident, but it would also make "this role has no cities" unrepresentable, and the index
  could never unset a facet.
- Touching the full reindex.

## Decisions

### A separate `RoleClusterGeo`, not a widened `RoleClusterCount`

The two questions are asked at the same place with the same key, so one round trip is tempting.
Rejected: `RoleClusterCount` also serves the single-job detail read, which does not want the
geography and should not pay for three `array_agg(DISTINCT …)` per request. Keeping them
separate also mirrors the existing `…CountsAll` / `…GeoAll` pairing, so the four queries read as
two pairs rather than three shapes.

### The count already answers whether the geography query is needed

`mass_count` is the number of **open** rows in the cluster. At `mass_count <= 1` the canon is
the only open row, so the cluster union is precisely the canon's own geography and
`MergeClusterGeography` would be a no-op. Skipping the lookup there is exact, not a heuristic,
and it keeps the added cost off every singleton role — which is most of the catalogue.

When the count lookup itself fails, the callers already degrade to `(1, 1)`. That now also means
"skip the geography", which is the same conservative direction: the push carries the canon's own
geography, exactly as today, and the next full reindex repairs it.

### `RoleClusterGeo` mirrors `RoleClusterGeoAll`'s semantics exactly

Same LATERAL-unnest-then-aggregate shape, same `<> ''` filtering, same open-rows-only scope. It
differs in one way beyond scope: it carries no `HAVING`, so it always answers with exactly one
row. An unknown cluster aggregates over no rows and yields SQL `NULL` arrays, which pgx scans
into nil slices; `MergeClusterGeography` treats those as "leave this facet alone" because
`unionSorted` gates on `len(extra) == 0`, which nil satisfies. A singleton is different and
worth stating plainly: it answers with the canon's *own* geography, so merging it is a
self-union and a no-op — the callers skip it anyway on `mass <= 1`.

## Risks / Trade-offs

- **One extra query per indexed row in a multi-row open cluster.** → Bounded by the same
  condition that makes it necessary, and ingest already pays a per-row `RoleClusterCount`, so
  the shape of the cost is established. Singletons pay nothing.
- **The three sites now each carry five more lines of near-identical code.** → Accepted
  deliberately; see Non-Goals. The follow-on is now concrete rather than speculative, which is
  the better time to decide it.
- **A cluster whose geography changes without any of its rows being re-pushed** still waits for
  the reindex. → Unchanged by this work; the incremental path has always been per-row.
- **PR #1385 is open against the same file** (`internal/linkimport/linkimport.go`), in the
  upsert mapping rather than the index push. → Whichever lands second rebases; no semantic
  conflict.

## Migration Plan

None. New read-only query, no schema change, no backfill. The next reindex would have repaired
the narrowed documents anyway; this stops them being narrowed in the first place.

## Open Questions

None.
