## Context

The jobs feed orders by freshness, salary, or keyword relevance. None of those
know who is reading. The embedding-backed "recommended" sort was removed in
`e52f28a6` with `/recommendations`; `web/src/lib/facetModel.test.ts` still guards
that a legacy `?sort=cv` link degrades quietly rather than erroring.

Two things already exist and shape the design:

- `internal/candidate/jobmatch` scores ONE job against a profile — exact/adjacent/
  missing plus a coverage percent. It is deterministic, LLM-free, and serves the
  per-job match bar. It is not an ordering: applying it to a catalogue means
  scoring a window, and a window has to be assembled somehow.
- `insights_facet_stats` records how many open jobs name each skill, maintained by
  `cmd/rollup-facets` for the public `/open` page.

The catalogue is 1 360 899 indexed documents. The facet filters live in
Meilisearch, so any ordering that cannot compose with them in one query forces
either a SQL re-implementation of every filter or a window-and-rerank.

This design follows a spike (2026-08-27) that measured four approaches on a
throwaway Meilisearch v1.49. Its numbers appear under Decisions.

## Goals / Non-Goals

**Goals:**
- Order the whole catalogue by skill overlap, composing with existing facet
  filters in one engine query, with ordinary pagination.
- Reward both absolute overlap and coverage share; weight rare skills higher.
- Cost no inference: no embedding model, no LLM, no AI credits, nothing on the
  request path but arithmetic.
- Degrade silently for any caller who cannot be served it.

**Non-Goals:**
- Replacing `jobmatch.Compute`. It stays the per-job explanation; this is the
  ordering of many. They answer different questions and may legitimately disagree
  at the margin.
- Semantic similarity. Adjacent-skill reasoning belongs to `skilladjacency` and
  the per-job bar, not to the feed order.
- Resolving the host2 disk situation. That gates deployment and is tracked
  separately.
- Changing any existing sort, filter, or wire shape.

## Decisions

### Skills are a finite dictionary, so the vector is arithmetic

`internal/dict/skilltag` resolves 534 word aliases and 348 phrase aliases into
**749 distinct canonical slugs**. A vector over a known, closed vocabulary needs
no model: position `i` holds the weight of skill `i`.

The cosine of two such vectors expands to exactly the intended ranking:

```
cos(A, B) = Σ idf(s)² over s ∈ A∩B  /  (‖A‖ · ‖B‖)
```

Numerator: weighted overlap. Denominator: penalises both the one-tag vacancy and
the thirty-tag requirements dump.

*Alternative — a real embedding model over skill text.* Rejected on behaviour, not
cost: an embedding scores `java spring kotlin` as highly similar to
`go docker kubernetes` (both "backend topics"), so the feed fills with languages
the candidate does not know. A positional vector counts literal overlap and
cannot do that.

### Meilisearch ranks it, via a `userProvided` embedder

The engine accepts a vector we compute (`source: userProvided`) and orders by
cosine. Measured: vector ranking plus `country = "DE" AND seniority = "senior"` in
one query, and `offset=400` under that filter in 20ms. recall@20 against the exact
cosine: 95%.

*Alternative — send the skills as the search query.* Ranking rule `words` looks
like "how many query terms matched", but Meilisearch **ANDs** query terms and
drops them from the end, making the first skill an implicit requirement. Spiked
with 9 documents, 6 overlapping a 20-skill profile: `matchingStrategy: "last"`
found 3, `"all"` found 0, `"frequency"` found 0. A vacancy overlapping only on the
profile's 15th skill was unreachable, though a single-term query found it
instantly. Adding a filter alongside does not rescue the dropped documents.

*Alternative — filter in Meilisearch, rerank a window in Go.* Works and is cheap
(1000 light documents fetched and scored in 19ms), but the window can only be
assembled by freshness, so an ideal match two months old never enters it, and
pagination gains a hard ceiling.

*Alternative — a separate lightweight skills index.* Measured directly: a 10.7 MB
light index and a 206.8 MB fat index with realistic descriptions returned a
1000-document window in 18.9ms and 17.5ms. Meilisearch does not read fields that
were not requested, so the second index buys nothing and costs synchronisation.

*Alternative — pgvector.* Moving the ranking to Postgres means re-implementing
every facet filter in SQL.

### Positions are permanent; weights are not

A stored vector records values by position. `mine-skill-dictionary` grows the
dictionary regularly, so if a position were an index into a sorted list, one added
skill would shift every later position and silently invalidate 1.36M stored
vectors — no error, just a feed that starts ranking wrongly.

So the registry is generated and **append-only**, and the generator cannot
reorder. `Dimensions` is declared at 1024 against 749 assigned, leaving headroom.

Weights are the opposite: they express catalogue rarity, drift as it changes, and
a stale one only nudges the order. They come from `insights_facet_stats` — already
populated, so no new worker.

### The weights are a parameter of document construction, not an attachment

`FromJob` changes signature to take them. The sibling precedent (`doc.Reality`,
attached by the caller) exists because reality needs a clock and cluster counts;
weights are just a value. Threading them through the signature makes the compiler
catch an indexer that forgets — a document silently missing its vector would drop
out of the match feed with nothing raised anywhere.

### `sort=match` degrades, never errors

`/jobs/search` stays public and gains optional auth. Every reason the sort cannot
be served — no session, no profile, no skills, no recognised skills, no weights —
falls back to the default ordering. A shared or saved link carrying `sort=match`
must survive being opened by someone it cannot be served for; the jobs list
already ignores unknown filters for the same reason.

One subtlety: when the vector is present the request must carry **no** attribute
sort directive, because an explicit sort takes precedence over vector ranking in
the engine and would silently discard the requested order.

## Risks / Trade-offs

**[Compression is unavailable — the index grows ~10 GB]** → No mitigation exists;
it is a cost to accept or reject. Measured at 50k documents and extrapolated to
1.36M:

| variant | dims | growth | recall@20 | rare-skill hits |
|---|---|---|---|---|
| IDF, unquantized | 749 | +7.5 GB | **95%** | 15/20 |
| the same at the declared 1024 | 1024 | +10.2 GB | 95% | 15/20 |
| IDF, binaryQuantized | 749 | +2.0 GB | **10%** | 0/20 |
| plain binary, quantized | 749 | +2.0 GB | 10% | 0/20 |
| IDF via dimension repetition | 2660 | +2.4 GB | 25% | 0/20 |
| random projection | 128 | +2.8 GB | 65% | 12/20 |
| random projection | 256 | +4.4 GB | 60% | 20/20 |

Quantization does not degrade the result, it destroys it. The vectors carry 2-12
non-zeros out of 749, so a sign-bit quantiser leaves 749 bits in which nearly
every zero agrees across every document; resolution collapses and the rare skills
the weighting exists to surface vanish entirely.

**[The 1024-dimension headroom costs ~2.7 GB of that]** → Storage scales with the
declared width whether or not the tail is occupied. Worth revisiting at
implementation time against the measured index size: ~800 buys most of the room
for a third of the space, at the cost of a re-declaration (and rebuild) sooner.

**[Rebuilds get materially slower]** → Measured ~5x per document (50k docs: 88s
with vectors, 18s without). The cost is HNSW graph construction, so it lands on
full rebuilds, not on incremental `search_outbox` pushes. `freehire-reindexw` has
already outgrown its slot once; this must be scheduled deliberately rather than
dropped into the ordinary timer.

**[Deployment is blocked today]** → `cmd/reindex` refused to run on 2026-08-27
09:15 UTC (62 GiB free against a 70 GiB floor), and an embedder cannot be added to
a live index incrementally. Also worth naming: `document.go` already caps indexed
descriptions at 1000 runes *specifically* to keep rebuilds inside this budget, so
the team has paid search quality for this space once already.

**[Ordering the engine produces and the per-job bar reports can differ]** →
`jobmatch.Compute` credits adjacent skills; the vector counts literal overlap. A
card may read a coverage percent that does not monotonically track its position.
Accepted: they answer different questions, and collapsing them would mean either
teaching the bar to lie or dropping adjacency from the explanation.

**[A stale weight snapshot skews the order]** → Bounded and self-correcting: a
vacancy indexed before a weight shifts carries a slightly stale vector until its
next write. Only a *position* change corrupts, and the registry forbids that.

## Migration Plan

1. Ship the code. Nothing changes at runtime: no embedder is declared until the
   settings patch, and `sort=match` degrades to the default feed until vectors
   exist.
2. **Patch the live index settings before deploying a binary that queries the
   vector.** A vector search against an index with no such embedder is a 400 from
   the engine, surfacing as a failing `/jobs/search` for everyone who selected the
   sort. Same ordering hazard `role_type` documents at `client.go:565-570`.

   The blast radius is bounded by the dark launch rather than by the ordering alone:
   with `PUBLIC_MATCH_SORT` off, nobody can select the sort from the UI, so getting
   this order wrong costs a hand-typed `?sort=match` rather than the live feed. That
   is the safety margin, not an excuse to reorder the steps.

   Note that documents are safe in either order: an index with no declared embedder
   accepts `_vectors` on incoming documents without complaint (verified against a
   live engine), so the indexers can write vectors before the settings land.
3. Resolve disk, then run a full rebuild — scheduled deliberately, not via the
   ordinary timer. Until it completes the sort returns only the handful of
   postings re-indexed since.
4. Verify by hand: `?sort=match` is honoured from step 1 onward regardless of the
   SPA flag, precisely so the ordering can be checked on production before anyone
   can click it.
5. Set `PUBLIC_MATCH_SORT=1` and restart the web server.

**The SPA ships dark.** The sort control is hidden behind a runtime flag
(`web/src/lib/features.ts`, read from `$env/dynamic/public`) that defaults OFF.
Between steps 1 and 5 the feature is fully deployed and fully invisible: a
near-empty match feed reads as a broken product, not a new one, so the control
only appears once someone has confirmed the rebuild landed. Because the flag is
read at runtime, revealing it is an env edit plus a restart — no rebuild of the
SPA, no redeploy.

**Rollback:** remove the sort option and stop sending vectors. The stored vectors
are inert — they cost disk and nothing else. Dropping the embedder declaration
requires another rebuild, so rollback is "stop using it", not "remove it".

## Open Questions

- **Declare 1024 or ~800 dimensions?** Deferred to implementation: the constant
  lives in one place, and the decision wants the measured index size, which only
  exists once the embedder runs against real data.
- **Should the SPA hide the option, or show it disabled with an explanation, for a
  signed-in visitor whose profile has no skills?** Hiding is assumed; showing it
  with a prompt to fill the profile is plausibly better for conversion and is a
  product call, not a technical one.
