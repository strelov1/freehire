# Skill vectors

## Scope
Turns a set of canonical `skilltag` slugs into a fixed-width vector, so the search
engine can order the whole catalogue by cosine against a candidate's skills instead
of the application scoring a window by hand. Pure and I/O-free: no LLM, no embedding
model, no network. Skills are a finite dictionary, so the vector is arithmetic.

## The one rule everything turns on

> **A position is permanent. A weight is not.**

`registry.go` is generated and APPEND-ONLY: a skill's index in it IS its vector
position. Reordering or removing one invalidates every vector already stored in the
search index — silently, with no error raised anywhere, because a stored vector
records values BY POSITION. The feed simply starts ranking wrongly.

Weights are the opposite. They express catalogue rarity, they are expected to drift
as the catalogue changes, and a stale one nudges the order without corrupting
anything.

Regenerate after the dictionary grows (see the `mine-skill-dictionary` skill):

```bash
go generate ./internal/dict/skillvec/
```

then commit the result. The generator only appends — it cannot reorder — and it is
idempotent, so a re-run with an unchanged dictionary writes a byte-identical file.
A skill REMOVED from the dictionary keeps its position too: reclaiming the slot
would let a future skill inherit a retired one's position, and every vector written
before that reuse would read the new skill's weight at the old skill's slot.

`TestRegistryCoversEveryCanonicalSkill` is what stops a newly mined skill from
quietly ranking as absent — it has no position until the registry is regenerated.

## Dimensions
`Dimensions` (1024) is wider than the dictionary (749) so growth needs no
re-declaration. It is NOT free: Meilisearch stores the declared width whether or not
the tail is occupied, and at catalogue scale each 256 dimensions costs roughly 2.5 GB
of index. Changing it forces a full rebuild, and until that rebuild finishes the
index rejects every document carrying the new width.

## Weights
`Weights` holds one factor per position, derived from how many open jobs name each
skill (`insights_facet_stats`, populated by `cmd/rollup-facets` — no worker of our
own). Rare skills weigh more: an overlap on `git` says nothing, an overlap on
`erlang` says a great deal.

```text
idf(s) = ln((maxCount + 1) / (count(s) + 1)) + 1
```

The scale is anchored on the **commonest skill in the snapshot**, not on a catalogue
size. The textbook shape divides by the document count, but that number is not in this
package's reach, and the obvious substitute — the sum of the counts — is wrong in a way
that matters: a job naming ten skills contributes to ten of them, so the sum grows with
catalogue breadth and flattens the very contrast the weighting exists to create.

Two more deliberate choices:
- a skill absent from the counts is treated as **maximally rare**, not weightless —
  it is either newly mined or genuinely obscure, and both warrant weight;
- the result is floored at 1, so a skill every posting names still contributes
  something rather than vanishing from the vector entirely.

`Ready()` separates "no snapshot loaded" from "this job has no skills". Callers must
not conflate them: the first is an absence of knowledge and leaves a stored vector
alone; the second is knowledge of an absence and clears it.

## Why `Vector` returns nil rather than a zero vector
A zero vector is not "no opinion" — it is a document Meilisearch rejects, and a query
that ranks against nothing. So `Vector` reports an absence: no weights loaded, no
skills given, or no slug recognised all yield nil.

What the caller does with that nil depends on WHY (see `Ready()`): with weights loaded
it writes an explicit clear, because the index merges document fields and an omission
would leave a stale vector ranking a job by skills it no longer has; with no weights it
omits the field and leaves whatever is stored alone.

## Why the cosine is the score
Vectors are unit length, so

```text
cos(A, B) = Σ idf(s)² over s ∈ A∩B  /  (‖A‖ · ‖B‖)
```

The numerator is the weighted overlap; the denominator penalises both the one-tag
vacancy (which would otherwise score 100% coverage) and the thirty-tag requirements
dump (which would otherwise win on volume). `TestCosineOrdersOverlapAndCoverage`
pins that worked example — **at equal rarity**, which is the qualifier that matters.
A vacancy asking only for a scarce skill the candidate holds CAN outrank a broader
match on ubiquitous ones, and should: that is what weighting by rarity means. Do not
"fix" that by capping the weights.

## Not to be confused with
`internal/candidate/jobmatch` scores ONE job against a profile and credits adjacent
skills via `skilladjacency`; it serves the per-job match bar. This package orders
many jobs and counts literal overlap only. The two may disagree at the margin — a
card's coverage percent does not have to track its position in the feed — and that
is accepted rather than reconciled: they answer different questions.
