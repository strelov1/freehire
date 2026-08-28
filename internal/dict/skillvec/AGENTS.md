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

## Why there are no rarity weights
An earlier version weighted each skill by how rare it is in the catalogue, so that
matching `erlang` counted for more than matching `git`. That is a defensible idea and it
is **gone**, because it is incompatible with the ordering the feed promises.

The promise is that the feed reads as one descending run of coverage: every 100% match,
then every 95%, and so on. Rarity weights break it — an IDF spread of 1..13 is far wider
than the gap between 100% and 95%, so a 93% match on scarce skills outranks a 100% match
on ordinary ones. Measured on production, the top forty carried fourteen such
inversions, the worst a 33-point drop, and the feed read as random.

Damping did not rescue it. Swept over real data, a factor of 0.05 was clean across forty
results and broke by a hundred; only removing the tilt entirely holds the order at
depth. So **every recognised skill contributes the same amount**, and what a posting is
worth is decided by how much of it the reader covers.

The consequence is worth stating plainly: 100% of `[git, sql]` ranks with 100% of
`[erlang, rust]`. Within one coverage band the posting asking for MORE skills wins,
since engaging twenty you hold beats engaging six.

`TestOrderIsStrictlyDescendingCoverage` is the guard. If rarity ever comes back, it will
fail — and it should.

## Why the cosine is the score, and why it needed a ballast
Vectors are unit length, so the cosine is the overlap over the product of lengths. That
alone is NOT enough. When a vacancy sits almost entirely inside a large profile the
expression collapses to roughly `sqrt(overlap) / ||A||` — it rewards the SIZE of the
overlap, and the denominator only penalises the vacancy's skills the reader does NOT
hold. So a posting listing 79 skills of which the reader holds 63 beats one listing 5
they hold entirely. Against a real 162-skill profile the entire top ten was postings
carrying 52-92 skills, out of a catalogue whose median is **7**.

**The fix is `ballastPosition`:** one slot the profile never sets. It contributes
nothing to the numerator and lengthens the job's vector, so the score reduces to about
`overlap / |B|` — coverage. Hence two constructors, `JobVector` (with ballast) and
`ProfileVector` (without). Swapping them silently restores the defect, and
`TestFromJobUsesTheJobSideConstructor` catches that.

**The floor of six skills is load-bearing.** A single tag the reader holds is 100%
covered, so without a floor the feed fills with one-skill postings — swept over real
data, the top ten became single-skill nursing vacancies.

Constants come from a sweep over a stratified production sample (k in [2,12], floor in
[5,12]); results plateau from k=4, so the shipped setting is the gentlest that reaches
it.

## Not to be confused with
`internal/candidate/jobmatch` scores ONE job against a profile and credits adjacent
skills via `skilladjacency`; it serves the per-job match bar. This package orders
many jobs and counts literal overlap only. The two may disagree at the margin — a
card's coverage percent does not have to track its position in the feed — and that
is accepted rather than reconciled: they answer different questions.
