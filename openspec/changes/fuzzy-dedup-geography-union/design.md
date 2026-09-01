## Context

Three deduplication passes run in `cmd/reindex`'s `refreshDuplicateMarkers` — role clusters,
aggregator suppression, then the fuzzy-description collapse — each writing its own owned column
(`duplicate_of_role`, `duplicate_of_aggregator`, `duplicate_of_fuzzy`). Migration 0115's trigger
derives `duplicate_of = COALESCE(aggregator, role, fuzzy)`, and a row with `duplicate_of` set is
excluded from the search index by every writer and reaper.

Separately, a canonical row's search document is widened with the geography of the rows it hides.
That widening is keyed by `(company_slug, role_fingerprint)` and is served by three read-only
queries — `RoleClusterGeo`, `RoleClusterGeoAll`, `RoleClusterGeoFor` — consumed by
`cmd/reindex/main.go:497`, `cmd/search-drain/indexer.go:129` and
`internal/ingest/linkimport/linkimport.go:336`. A fourth consumer of the same grouping,
`ListRoleClusterCopies`, backs `GET /jobs/:slug/copies` and the detail page's "Other locations (N)"
tab.

The fingerprint key cannot express what the fuzzy pass does. `RoleFingerprint` excludes `location`
but hashes the description, so a per-city variant whose body names its city gets a different
fingerprint; that is exactly the population the fuzzy pass exists to catch (its 0.9 threshold was
calibrated on per-city variants scoring 0.95–1.00). Every row the fuzzy pass suppresses therefore
has a fingerprint its canon does not share, the geography lookup misses, and the hidden cities
leave search with the hidden row. Issue #2225 reproduces this twice.

A second defect is independent of the first. The fuzzy pass's candidate queries filter
`duplicate_of IS NULL`, so a fuzzy-marked row is invisible to every later run, and
`MarkFuzzyDuplicatesForCompany` can only ever write a non-NULL canon id. Before 0114/0115 the role
recompute wrote `duplicate_of` directly and did reverse the fuzzy pass; two comments in `jobs.sql`
(1630, 1735) still promise that. Today a fuzzy marker is permanent.

## Goals / Non-Goals

**Goals:**

- A searchable row's geography facets cover every open row it represents, whichever pass
  suppressed them, including through a chain of markers.
- `/jobs/:slug/copies` lists the same set the geography union covers, so a city a user can filter
  to is a city they can reach.
- The fuzzy pass re-decides its own markers every run and releases the ones that no longer hold.
- One grouping concept instead of two, so a fourth dedup pass inherits geography for free.

**Non-Goals:**

- Changing what the fuzzy pass merges. The 0.9 threshold, the `fuzzyMaxBucket = 200` cap, the
  company+title bucket and the non-transitive canon comparison are untouched.
- New UI. `JobRelated.svelte` and the `/jobs/:slug/copies` page already render whatever the
  endpoint returns; a "+N cities" badge on the feed card is explicitly out of scope.
- Widening the geography served by `/api/v1/jobs` and `/api/v1/jobs/:slug`. Those read Postgres
  rows and carry each row's own facets; the asymmetry with search-served documents is pre-existing
  and is not addressed here.
- Any schema change. `jobs_duplicate_of_idx` (migration 0012) already indexes the traversal.

## Decisions

### Key the union by owner job id, not by `(company_slug, role_fingerprint)`

The question the writers actually need answered is "which open rows does this searchable row
represent?", and `duplicate_of` already records it. Traversing it subsumes the role-cluster case
(members point at their canon) and adds the fuzzy, aggregator and multi-hop cases.

The lookup signature simplifies as a side effect: a writer passes `job.ID` instead of a slug and a
fingerprint, and the `RoleFingerprint.Valid` guards around the geography merge disappear.

*Alternative considered — add a parallel fuzzy-keyed union next to the fingerprint one.* Rejected:
two mechanisms in each of four consumers, and the next dedup pass would need a third.

*Alternative considered — materialize the union into new `search_*` array columns on `jobs`.* The
read side becomes trivial, but it adds three columns, a write pass over the catalogue and a
staleness question, to solve a problem the read side can answer directly. Rejected as
over-engineering, and it is the seam to reach for only if the recursive read measures badly.

### Seed the traversal from searchable rows and walk toward members

The recursive term walks `child.duplicate_of = parent.id` downward, seeded from rows that are open
with `duplicate_of IS NULL`. Because the seed is exactly the set of rows that are nobody's
duplicate, a cycle among markers has no NULL-marked entry point and is simply never visited — the
traversal is cycle-safe by construction rather than by a path-tracking guard. A depth bound stays
in as a backstop against a future pass that could seed differently.

Walking upward from each duplicate to its terminal owner was the alternative. It touches fewer rows
but needs an explicit termination test per row and a real cycle guard, and it yields
`member → owner` pairs that still have to be inverted. Downward is the simpler statement.

### One recursive definition, three seeds

The whole-catalogue rebuild, the drain wave and the single-row link import need the same closure
over different seeds. They become three sqlc queries sharing one recursive body — differing only in
whether the seed is "every owner", "owners in this id set" or "this one id". Keeping the body
identical is what stops the three from drifting the way the three `RoleClusterGeo*` queries could.

### `/copies` resolves the anchor to its owner first

Today the endpoint groups by the anchor's own fingerprint, so opening a suppressed posting shows
that posting's role cluster — a fragment. Resolving the anchor to its ultimate owner and listing
that owner's closure is both the fix for the fuzzy case and a strict improvement for the existing
one. `meta.total` keeps its current meaning: `COUNT(*) OVER()` over the closure, pre-LIMIT.

### The fuzzy pass releases its own markers

Mirror `RecomputeRoleDuplicatesForCompanies`. Candidate selection changes from `duplicate_of IS
NULL` to "not claimed by an exact pass" (`duplicate_of_aggregator IS NULL AND duplicate_of_role IS
NULL`), which is what the pass's own doc comment already claims it does; and
`MarkFuzzyDuplicatesForCompany` takes the full candidate set alongside the assignment so it can
write NULL for a row that no longer clusters. The existing transition bookkeeping already inserts
into `search_outbox` on duplicate→canonical, so a released row returns to search with no new code.

Re-admitting marked rows makes buckets larger. `clusterBucket` picks the lowest id as canon and
compares only against it, so the second run reproduces the first run's assignment exactly — the
`IS DISTINCT FROM` guard then makes it a no-op write. `fuzzyMaxBucket` still caps the work.

Reversal must belong to the fuzzy pass itself: the other two passes write different columns and the
`COALESCE` in migration 0115 keeps surfacing a fuzzy marker no one can clear.

## Risks / Trade-offs

- **The whole-catalogue recursive closure is slower than one grouped scan, against ~1.4M open
  rows.** → Measure `EXPLAIN ANALYZE` on a prod-sized dataset before rollout and record the number
  in the change. The seed narrows to rows that actually own duplicates, which is far smaller than
  the current query's group set, so the expectation is neutral-to-faster; but a dedup pass here
  once ran 23h against a 12h unit timeout, so this is measured, not assumed.
- **A larger fuzzy candidate set loads more descriptions per company.** → Descriptions are still
  only fetched for buckets that survive the size filter, and the cap is unchanged. Bucket
  membership grows by the rows the pass previously hid from itself — bounded by what it merged.
- **Releasing stale markers returns rows to the index in one burst.** → They flow through
  `search_outbox`, which is drained in batches by the existing worker; the transition is the same
  one a released role duplicate already takes. Expect a one-off bump in catalogue counts and say so
  when the change ships.
- **A writer that forgets the union silently narrows the index rather than failing.** → The
  hazard is unchanged but the surface moves; the existing warning in
  `document.go:188` must be updated to name the closure, and a test must cover each of the three
  writers rather than only the rebuild.
- **`/copies` membership widening is user-visible.** A posting that never appeared under "Other
  locations" starts appearing. That is the requested behaviour, not a regression, but it changes
  what an existing page shows without a UI diff.

## Migration Plan

No schema migration. Rollout order matters because the widened facets only reach Meilisearch
through a rebuild or a drain push:

1. Deploy.
2. `systemctl stop freehire-reindexw.timer` — a scheduled rebuild colliding with the manual one
   queues behind it in Meilisearch's serial task queue and looks like a hang.
3. `REINDEX_DEDUP_ONLY=1` — releases stale fuzzy markers and queues the freed rows. No Meilisearch
   client, so `search-drain` need not be paused.
4. Full `make reindex` — the only thing that rewrites the geography of every existing document.
5. Re-verify the four URLs from issue #2225: the two affected searches must return `total: 1`, the
   two controls must stay `total: 1`.
6. Restart the timer.

Rollback is a redeploy of the previous binary plus a full reindex; no data is destroyed, since
every marker the release step cleared is re-derived by the next dedup run.

## Open Questions

- What depth bound? A role→fuzzy chain is 2 and aggregator→role→fuzzy is 3; the bound should leave
  headroom without inviting an unbounded walk. Settle it when the query is written and state the
  reasoning in the query comment.
- Does `linkimport` still need a single-row closure query, or can it share the by-id-set one with a
  single-element argument? Prefer sharing if the plan is the same; decide with `EXPLAIN`.
