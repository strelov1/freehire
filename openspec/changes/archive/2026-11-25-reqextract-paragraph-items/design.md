## Context

See proposal.md - Why. `Derive`'s walk carries one piece of state, `priority`
(the section currently open), and previously closed that section the moment it
saw ANY node it could not read as a heading or a list — including a genuine
paragraph. That rule is what protects a benefits list two elements down from
being misread as requirements (see AGENTS.md's "Always true" bullets), so it
cannot simply be dropped; it has to be replaced with something that still tells
a `<p>`-per-item requirements section apart from prose that explains the
section away, using only what a single-pass walk can see.

The measurement behind this change: a `TABLESAMPLE`-based real-data check
against `tbank.ru` (25 postings pulled live, 2026-09) found every "Требования"
heading followed by a run of 3-10 `<p>` items, never a `<ul>`/`<ol>`, matching
the 683-heading count already recorded in AGENTS.md's Limitations section from
the vocabulary work that surfaced this gap.

## Goals / Non-Goals

**Goals:**
- Read a `<p>`-per-item requirements section the same way a `<ul>`/`<ol>`
  section is read today: gated by the same heading vocabulary, same priority
  rule, same bound.
- Preserve every existing behavior byte-for-byte: no regression on any of the
  pre-existing `TestDerive` cases, `MaskPreferred`, or the benefits-list
  exclusion.
- Resolve the genuine three-way ambiguity a single paragraph creates (is it an
  item, a lead-in, or prose closing the section?) from structure alone, with no
  new vocabulary and no heuristic tunable beyond the existing "≥2 items" rule.

**Non-Goals:**
- Clustering or ordering paragraph items by any signal beyond document order.
- Reading a `<p>`-per-item PREFERRED section differently from a required one —
  the same buffer and the same priority rule apply to both.
- Extending this to any other repeated-inline-element shape (`<span>` runs,
  `<br>`-separated lines within one block) — only sibling block-level text
  blocks are in scope, because that is the shape actually measured.

## Decisions

**A deferred buffer, not an immediate classification.** A candidate paragraph
cannot be classified as "item" or "not item" from its own content — the
tbank.ru shape and a plain explanatory sentence after a heading look identical
at the node level. The only thing that disambiguates them is what follows:
more paragraphs (commit), a list (the list wins instead), or nothing before the
section's natural end (commit if ≥2, drop if 1). So the walk defers the
decision: every over-length text block under an open section goes into
`pending`, and `pending`'s fate is decided at the next resolving event rather
than at the paragraph itself. Considered and rejected: a two-node lookahead
(peek at the next sibling before deciding) — this only works within one parent
element and a posting's own markup nests paragraphs inconsistently (sometimes
siblings, sometimes each wrapped in its own `<div>`), so a lookahead would need
to skip wrapper boundaries anyway, which is exactly what the single-pass walk
already does for every other case. Buffering costs nothing else — the walk stays
single-pass and its existing state (`priority`) is unchanged in shape, just
joined by two more decision-only fields.

**The ≥2 threshold, not ≥1.** A section with exactly one over-length paragraph
under a heading is exactly the shape the ORIGINAL negative test in this package
existed to guard: "a heading with prose after it yields nothing" — a posting
that states its heading and then explains the role in one sentence before
moving on, not a requirements list. Nothing in the markup tells that sentence
apart from the FIRST item of a genuine multi-item run; only a second paragraph
does, by existing at all. Requiring two therefore keeps the original guarantee
intact for the single-paragraph case while opening the multi-paragraph one —
and it matches the real data: every sampled tbank.ru posting had 3 or more,
comfortably clear of the boundary.

**`pendingHasProse`, not "any pending buffer discards on a list".** A short
unrecognized inline line (a lead-in like `<p>You will need:</p>`, or a bare
spacer `<p></p>`) must NOT prevent a following list from being read — that
behavior already existed before this change (the walk already tolerated
lead-in lines between a heading and its list) and regressing it would break
real postings that use exactly that shape. Only a line long enough to have
failed the heading-length test (real prose) should cause a following list to
be treated as belonging to a DIFFERENT section. `pendingHasProse` is the one
bit that keeps these two apart: set only by the over-length branch, read only
by the list-found branch.

**Commit at a heading transition, a table, AND the document's end — one
function, `flushPending`, not three inline copies.** All three are "the
section's fate is now decided, and no list was found" — the same operation
with a different closing priority argument (the section's own priority for a
heading/table transition, whatever `priority` holds at `walk`'s return for the
document's end, which is the same variable). Duplicating the ≥2 check at three
call sites was rejected as the obvious source of a future one-off divergence
if only two of the three were updated later.

## Risks / Trade-offs

- **[Risk] A posting genuinely explaining a section in exactly two long
  sentences (not stating two requirements) now reads both as items.** →
  [Mitigation] This is the same class of imprecision the existing dictionary
  gate already accepts everywhere else in this package (see AGENTS.md: "a
  missed section is a blank space, while a benefits list under a Requirements
  heading is a false claim the reader cannot detect") — two sentences read as
  two requirements is the same direction of error the ≥2 threshold already
  chose to accept over the alternative (missing every real multi-paragraph
  section). Not measured as a real occurrence in the tbank.ru sample or the
  existing test corpus.
- **[Risk] `internal/job/reqextract/AGENTS.md`'s "Measuring coverage" figures
  (28.0% list-shaped, 29.3% combined with the model) now undercount, since
  they predate this fix.** → [Mitigation] AGENTS.md is updated to flag this
  explicitly rather than silently going stale; re-measuring with the same
  `TABLESAMPLE` method is left as a follow-up, not guessed at here.
- **[Risk] The stored catalogue does not benefit until re-derived.** →
  [Mitigation] Not this change's concern to trigger — `backfill-requirements`
  (see root AGENTS.md) is the existing one-off that re-reads `requirements_derived`
  for open postings and is idempotent; running it is an operational follow-up,
  same as any other `Derive` behavior change.

## Migration Plan

No schema change. `Derive` is called from the ordinary ingest write path
(`internal/job/job`'s `withDerived`) and from `backfill-requirements` — both
pick up the new behavior on deploy with no migration step. The pre-existing
catalogue is unaffected until `backfill-requirements` is re-run by an operator,
which is out of scope here (see Risks). No feature flag: the change is a
strict superset of what `Derive` already read (a `<p>`-per-item section that
previously yielded nothing now yields entries; every previously-yielding shape
is unchanged, confirmed by the full pre-existing `TestDerive` suite passing
unchanged). Rollback is a plain revert — no data written by this change is
irreversible or requires cleanup.
