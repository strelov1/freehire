# Sorting the jobs feed by skill match

**Status:** design approved 2026-08-27, not implemented
**Blocked on:** free disk on host2 (see "What it costs" — the rebuild this needs
currently refuses to run)

## The ask

Order the jobs feed by how well the vacancy's skills match the signed-in
candidate's, replacing the embedding-backed "recommended" sort that was dropped
in `e52f28a6` along with `/recommendations`.

## What "match" means

Ranking must reward **both** how many of the candidate's skills the vacancy
engages **and** what share of the vacancy's requirements they cover. Neither
half works alone:

| vacancy (profile = go, docker, k8s, postgres, grpc) | matched | coverage |
|---|---|---|
| `[go]` | 1 | 100% |
| `[go, docker, k8s, postgres, grpc]` | 5 | 100% |
| `[go, docker, k8s, postgres, aws, tf, kafka, redis, grpc]` | 6 | 67% |

Coverage alone floats the one-tag vacancy to the top. Absolute overlap alone
lets a 30-tag requirements dump tie a well-targeted posting. The intended order
here is C > B > A.

Additionally, **rare skills count for more**. An overlap on `git` says nothing;
an overlap on `erlang` or `sap-abap` says a great deal.

## The approach

A **749-position skill vector**, IDF-weighted, ranked by Meilisearch's own
vector search using a `userProvided` embedder.

No embedding model is involved. Skills are already canonical slugs from a finite
dictionary (`internal/dict/skilltag` — 534 word + 348 phrase aliases resolving to
**749 distinct canonical skills**), so the vector is arithmetic, not learned.

The cosine of two such vectors expands to exactly the ranking the section above
asks for:

```
cos(A, B) = Σ idf(s)² over s ∈ A∩B  /  (‖A‖ · ‖B‖)
```

The numerator is the weighted overlap; the denominator penalises both the
one-tag vacancy and the requirements dump.

### Why not an embedding model

A real embedding of skill text scores `java spring kotlin` as highly similar to
`go docker kubernetes` — both are "backend topics". The feed would fill with
languages the candidate does not know. A positional vector counts literal
overlap and cannot do that.

### Why Meilisearch and not pgvector

The facet filters (country, seniority, salary, work mode, …) live in
Meilisearch. Ranking anywhere else forces either a re-implementation of every
filter in SQL, or a window-and-rerank with a pagination ceiling. Meilisearch
applies the vector ranking and the facet filter in **one query**, with ordinary
pagination.

## Components

| piece | where | note |
|---|---|---|
| skill → vector position | new, `internal/dict/skillvec` | the position registry; see "The position registry" |
| IDF weights | existing `insights_facet_stats` (`facet='skills'`) | already populated by `cmd/rollup-facets`; **no new worker** |
| vacancy vector | `search.JobDocument._vectors.skills` | derived at index time, exactly like `Roles` / `AIArchetype` / `RoleType` |
| candidate vector | computed per request from the caller's profile skills | 749 floats, sub-millisecond |
| embedder declaration | `search` index settings | `source: userProvided`, `dimensions: 1024`, `binaryQuantized: false` |
| the sort itself | a new `sort` value on `/jobs/search` | requires auth; not offered to anonymous callers |

`jobmatch.Compute` is untouched and keeps serving the per-job match bar. It
stays the candidate-facing explanation of a single vacancy; the vector is the
ordering of many.

## The position registry

A skill's position in the vector must be **permanent**.

`mine-skill-dictionary` grows the dictionary on a regular cadence. If a
position were simply the skill's index in a sorted list, adding one skill would
shift every later position and silently invalidate all 1.36M stored vectors —
no error, just a feed that quietly goes wrong.

So positions are assigned once and never reused: a new skill takes the next free
slot. Dimensions are declared as **1024**, not 749, leaving 275 slots of
headroom. A test asserts the registry is append-only and covers every canonical
skill.

## What it costs

Measured on a throwaway Meilisearch v1.49 with 50k synthetic jobs over a
749-skill Zipf-distributed dictionary, then extrapolated to the live index
(1 360 899 documents):

| variant | dims | index growth @1.36M | recall@20 vs exact | rare-skill hits |
|---|---|---|---|---|
| **IDF, unquantized** | 749 | **+7.5 GB** | **95%** | 15/20 |
| **↳ the same at the declared 1024** | 1024 | **~+10.2 GB** | 95% | 15/20 |
| IDF, binaryQuantized | 749 | +2.0 GB | 10% | 0/20 |
| plain binary, quantized | 749 | +2.0 GB | 10% | 0/20 |
| IDF via dimension repetition, quantized | 2660 | +2.4 GB | 25% | 0/20 |
| random projection | 128 | +2.8 GB | 65% | 12/20 |
| random projection | 256 | +4.4 GB | 60% | 20/20 |

Every row was measured at 749 dimensions, the dictionary's current size. The
design declares **1024** for headroom (see "The position registry"), and storage
scales linearly with the declared width whether or not the extra slots are
occupied — hence the ~+10.2 GB row. Recall is unaffected: the spare positions are
zero in every vector. **That headroom costs ~2.7 GB, and given the disk situation
below it is a decision worth revisiting at implementation time** — a narrower
declaration (say 800) buys most of the room for a third of the space, at the cost
of a re-declaration (and therefore a rebuild) sooner.

**Compression is not available.** The vectors are sparse — 2-12 non-zeros out of
749 — so a sign-bit quantiser leaves 749 bits in which nearly every zero agrees
across every document. Resolution collapses, and the rare skills the IDF
weighting exists to surface disappear from the results entirely. Random
projection is honest but trades a third of the accuracy for under 3x the space.

Indexing is also **~5x slower** with vectors (50k docs: 88s with, 18s without).
That cost is HNSW graph construction, so it lands on a full rebuild, not on the
incremental `search_outbox` pushes.

### The disk problem this lands on

This is the one genuinely uncomfortable part of the design, and it is not
hypothetical:

- `cmd/reindex` refused to run on 2026-08-27 09:15 UTC — free space was 62 GiB
  against the 70 GiB `REINDEX_MIN_FREE_GB` floor.
- The `jobs` index is 15 GB of an 18.3 GB Meilisearch database. Adding 7.5 GB
  makes it ~25.8 GB, and a swap-rebuild transiently holds two copies.
- `document.go` already caps indexed descriptions at 1000 runes (`maxIndexedDescriptionRunes`)
  **specifically to keep a rebuild's footprint inside the host's free disk**. The
  team has already paid search quality for this budget once.

A full rebuild is mandatory to introduce the embedder — it cannot be added to a
live index incrementally. **Disk must be resolved before this ships**, and that
is separate work: on host2 Postgres holds 116 GB (`jobs` alone is 96 GB — 19 GB
heap, 63 GB TOAST, 13 GB indexes, over 10.2M rows), and the 70 GiB floor is
roughly four times the ~18 GB a swap actually needs.

## Behaviour

- **Anonymous caller:** the sort value is not offered. An explicit request for it
  is treated the way `/jobs` already treats an unknown filter (see
  `hire-jobs-list-ignores-unknown-filters`) — it degrades to the default feed
  rather than erroring, so a shared link never 400s.
- **Signed-in caller with no skills:** same degradation. A profile with no skills
  has a zero vector, which ranks nothing meaningfully.
- **Facet filters:** compose normally. Verified in the spike: vector ranking plus
  `country = "DE" AND seniority = "senior"` in a single query, and `offset=400`
  under that filter returned in 20ms.
- **Stale IDF:** weights come from a rollup that runs on its own schedule. A
  vacancy indexed before a weight shifts carries a slightly stale vector until
  its next write. This is drift, not breakage — the order moves a little, nothing
  becomes wrong. Only a *position* change would be corrupting, and the registry
  forbids that.

## Testing

- `internal/dict/skillvec`: the registry is append-only; every canonical
  `skilltag` slug has a position; positions are unique; the dimension count fits
  the declared 1024.
- Vector construction: a known skill set produces the expected weighted vector;
  an unknown slug is ignored rather than panicking; an empty skill set yields the
  zero vector.
- Cosine ordering: the worked example from "What match means" ranks C > B > A.
- Handler: anonymous and skill-less callers degrade to the default feed; the
  sort composes with facet filters.
- Index settings: a test asserts the embedder is declared with the dimensions
  the registry expects, mirroring the existing `settings_test.go` guards.

## Rejected alternatives

**Rank in Meilisearch by sending skills as the search query.** Meilisearch ANDs
query terms and drops them from the end, so the first skill becomes an implicit
requirement. Spiked with 9 documents, 6 of which overlapped the profile:
`matchingStrategy: "last"` found 3, `"all"` found 0, `"frequency"` found 0. A
vacancy overlapping only on the profile's 15th skill was unreachable, though a
single-term query for that skill found it instantly. Adding a filter alongside
the query does not rescue the dropped documents.

**Filter in Meilisearch, rerank a window in Go.** Works and is cheap (fetching
1000 light documents plus scoring them measured 19ms), but the window has to be
assembled by freshness — the engine cannot order by overlap — so an ideal match
two months old never enters it, and pagination has a hard ceiling.

**A separate lightweight skills index.** Measured directly: a 10.7 MB light
index and a 206.8 MB fat index with realistic descriptions fetched a
1000-document window in 18.9ms and 17.5ms respectively. Meilisearch does not
read fields that were not requested, so `attributesToRetrieve` on the existing
index gives the same cost without a second index to keep in sync.

**Rank in Postgres with pgvector.** The catalogue's facet filters live in
Meilisearch; moving the ranking to Postgres means re-implementing all of them in
SQL.
