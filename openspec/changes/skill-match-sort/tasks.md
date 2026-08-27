Step-by-step code, test bodies, and exact file lines live in
`docs/superpowers/plans/2026-08-27-skill-match-sort.md` (same branch). Each task
below is one TDD cycle: red, green, refactor, simplify, review, then tick.

## 1. The vocabulary

- [x] 1.1 ~~Expose the skill dictionary's canonical slugs as `skilltag.Canonicals()`~~ — **already exists** at `internal/dict/skilltag/labels.go:377`, feeding `Labels()` and the SPA's skill catalog through `cmd/gen-contracts`. It is strictly wider than planned, drawing from all five alias tiers (word, phrase, shared/resume/category-scoped acronyms) rather than two. Measured: all tiers together yield the same **749** canonicals, and none is reachable only through an acronym — so the vector width and every disk figure in `design.md` stand.

## 2. The vector

- [x] 2.1 Write the append-only registry generator (`internal/dict/skillvec/gen`) and generate `registry.go`; the generator appends unassigned canonical skills and never reorders
- [x] 2.2 Add `Dimensions`, `Position`, and `RegistrySize`, with tests that the registry has no duplicates, covers every canonical skill, and cannot outgrow the declared width
- [x] 2.3 Register `skillvec` in the `dict` block of `internal/platform/arch/layering/blocks.go` and confirm the layering guard passes
- [x] 2.4 Implement `Weights`, `WeightsFromCounts`, and `Weights.Vector` — IDF-shaped weights, unit-length vectors, nil for every unusable input, unknown slugs ignored, a duplicated skill counted once
- [x] 2.5 Write `internal/dict/skillvec/AGENTS.md`: why a position is permanent and a weight is not, what `Dimensions` costs, how to regenerate

## 3. Wiring the weights

- [x] 3.1 Add `search.LoadSkillWeights` reading the `skills` rows of `insights_facet_stats`; a snapshot with no skill rows yields the zero `Weights`, never an error

## 4. The indexed document

- [x] 4.1 Add `SkillEmbedder` and `JobDocument.Vectors` (`_vectors`, omitted when empty), and change `FromJob` to take the weights so an indexer that forgets fails to compile
- [x] 4.2 Update the three indexers (`cmd/reindex`, `cmd/search-drain`, `internal/ingest/linkimport`) to load the weights once per run and pass them; a weight-load failure logs and continues rather than aborting the run

## 5. The engine

- [x] 5.1 Declare the `userProvided` embedder in the index settings at `skillvec.Dimensions`, binary quantization off, with a test asserting all three
- [x] 5.2 Add `SearchParams.Vector` and have `Search` send it with the hybrid embedder directive at a semantic ratio of 1.0

## 6. The endpoint

- [x] 6.1 Attach optional auth to `/jobs/search` and build the caller's vector for `sort=match`, reading the weights through the handler's cache rather than per request
- [x] 6.2 Suppress the attribute sort directive whenever a vector is sent, so the engine does not discard the match ordering
- [x] 6.3 Cover all four request cases: eligible caller ranked by vector; anonymous, profile-less, and skill-less callers served the default feed with `200`; the sort composing with facet filters

## 7. The SPA

- [x] 7.1 **The feed had no sort control at all** — it went with `sort=cv`: `JobFilters` had no `sort` field and a test asserted the param was never serialized. So this restored the mechanism (field, URL round trip, a deliberately two-value vocabulary) rather than extending an existing one. An unrecognized value reads as the default, mirroring the backend.
- [x] 7.2 Offer the option in the sort selector to signed-in visitors whose profile has skills

## 8. Rollout documentation

- [x] 8.1 Record in `internal/search/search/AGENTS.md` both ordering hazards: the embedder must exist in the live index before a binary queries it, and the vectors only exist after a full rebuild
- [x] 8.2 Note in `CLAUDE.md`'s `reindex` bullet that a rebuild now also writes skill vectors, costs ~10 GB more, and runs materially longer

## 9. Dark launch

- [x] 9.1 Hide the sort control behind `PUBLIC_MATCH_SORT` (`web/src/lib/features.ts`), default OFF — the API honours `?sort=match` regardless, so the ordering can be verified on production before the control appears
- [x] 9.2 Document the flag and the reveal step in `web/.env.example`, the change's design, and `internal/search/search/AGENTS.md`
