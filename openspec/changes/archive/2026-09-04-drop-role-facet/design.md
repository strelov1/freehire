## Context

The `role` facet was built to answer "what job is this", deterministically, from a
title. It works — the dictionary is careful and its tests are good. The case against it
is not that it is wrong but that it is redundant and unused, and both halves of that
were measured rather than felt.

Redundant: of 1,200 role values, 47 repeat a specialization **identically** (all 47
return the same posting count to the digit), 979 are specialization × seniority, 8 are
a bare grade, and 166 carry their own name.

Unused: 894 of 54,870 searches over two days, against 8,782 for `category`. A year
earlier the same figure was 1.1%.

And thin where it is not redundant: seniority resolves for 24.3% of the catalogue
against 97.5% for specialization, so the 979 graded values are blind to three postings
in four.

What changed recently is the alternative. The suggestion dictionary that shipped this
week carries 21,176 mined posting titles — including every named role worth having,
written the way employers write them rather than the way a curated list guesses.

## Goals / Non-Goals

**Goals:**

- Remove the facet, its dictionary, and everything derived from it.
- Leave `category` + `seniority` answering the same questions on the same request.
- Let specializations become offerable suggestions, which they cannot be today.
- Keep a stale `role=` link honest: reported, not silently widened.

**Non-Goals:**

- Redirecting or rewriting old `role=` URLs. Decided explicitly: the param is dropped
  and reported through `meta.ignored_params`, the mechanism that already exists.
- Moving the 166 named roles into the specialization vocabulary. They belong to the
  title dictionary, which already has them and needs no curation to keep them.
- Touching `aiarchetype` or `roletype`. Both name roletag in a comment about shared
  doctrine; neither calls it. Verified before planning, because the proposal would have
  been much larger if they did.

## Decisions

### The AI filter emits two facets where it emitted one

`searchintent` offers the model a `role` enum built from `roletag.Catalog()`. It will
offer `category` and `seniority` instead — which is what a role slug decomposed into
anyway, so the interpreter says the same thing in the vocabulary the rest of the
request uses.

This is the one place where behaviour could get WORSE rather than simply narrower, so
it goes first and is checked against real prompts before the rest lands. "Senior backend
in Berlin" must still produce a senior backend search.

**Alternative considered:** leaving `role` in the interpreter's enum and translating it
server-side. Rejected — it keeps the vocabulary alive in the one place a model reads it,
which is where a retired concept quietly persists longest.

### `roles` leaves the index document, and the order matters

Removing a filterable attribute is the mirror of adding one. A binary that stops
declaring `roles` filterable while the LIVE index still has it is harmless; a binary
that still SERVES the facet against an index that no longer declares it 500s. So: stop
serving first, rebuild after. The clearance facet documented the opposite order for the
opposite reason, and the asymmetry is the whole hazard.

### The category-vs-role de-duplication is deleted, not inverted

The builder drops a category whose slug a role carries. With roles gone there is
nothing to collide with, and the rule's absence is what finally lets specializations
into the dictionary — today it holds zero of them, because every single one collided.

That is a small, pleasant consequence worth naming: the change removes a facet AND
makes the axis that replaces it reachable by typing, which it was not.

## Risks / Trade-offs

**A saved search or shared link carrying `role=` stops filtering.** → Reported in
`meta.ignored_params` rather than silently widening, and 1.6% of searches use the param
at all. The alternative — translating `role=senior_backend` into two params forever —
keeps a retired vocabulary alive in the one place nobody maintains.

**`role_mode=and` has no equivalent.** Two roles ANDed is one posting being two jobs,
which matched nothing useful; two specializations ANDed is the same shape. Nothing is
lost that produced results.

**The AI filter could get worse before it gets better.** → It ships first and alone, and
is checked against real prompts. If "senior backend" degrades, the rest of the change
waits.

**A named role nobody re-mines disappears.** A role like `founding_engineer` survives
only if enough postings are titled that way. → That is the honest answer to whether it
is a role people search for, and the frequency floor is a tunable if the answer turns
out to be "yes, but rarely written".

## Migration Plan

1. **The AI filter alone.** Emit `category` + `seniority`; verify against real prompts.
2. **Stop serving the facet**: remove it from the filter table, the facets endpoint,
   the OpenAPI contract and the filter modal. The index still declares the attribute,
   which is safe.
3. **Stop building it**: drop the kind from the suggestion builder along with the
   de-duplication rule, and rebuild the dictionary. Specializations appear.
4. **Delete the dictionary**: `internal/dict/roletag`, the generated contracts, the
   related-roles map, the web files that name the facet.
5. **Reindex**, which drops `roles` from every document.

Rollback: any step before 5 is a revert. After 5 the attribute is gone from the index
and coming back needs a rebuild, which is a scheduled job rather than an incident.

## Open Questions

None left. The one that was here — whether `internal/dict/classify` depends on roletag —
was answered by reading it: both mentions in `dictionaries.go` are comments explaining
where a gap belongs, not calls. Same for `aiarchetype` and `roletype`. The four packages
that looked coupled are coupled by doctrine and prose, not by code, which is why the
blast radius is six Go files rather than ten.
