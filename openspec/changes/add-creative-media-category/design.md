## Context

`internal/dict/classify` reads the TITLE only and never guesses. Its
`categoryTable` resolves in DECLARATION ORDER — the first alias that occurs as
a whole word wins — so vocabulary work here is placement work as much as it is
wording work. (`roletag` is the one that matches longest-alias-first; the two
tables do not share a rule, and treating them alike is how an alias ends up on
the wrong side of a collision.) The category it emits lands in
`jobs.category` — a plain text column with no `CHECK` constraint and no
Postgres enum; the vocabulary is enforced in Go by `vocab.CategoryValues`
alone. That is why a new category needs no migration, and why the whole cost of
this change sits in `backfill-derive` re-deriving existing rows.

Three consumers read the tech/non-tech split rather than the category itself:

- `jobderive.deriveIsTech` (`internal/job/jobderive/jobderive.go:249`) sets
  `is_tech = false` for any `NonTechCategories` member;
- the enrichment enqueue gate in `internal/platform/db/queries/jobs.sql`
  admits `is_tech IS TRUE` only;
- `cmd/prune/rule.go:110` treats every `NonTechCategories` member except
  `engineering_design` as removable.

`vocab_test.go` asserts that `TechCategories`, `NonTechCategories` and
`{"other"}` partition `CategoryValues` exactly, so a new value MUST join one of
the two sets — there is no "unclassified" option.

The previous split of this shape (`design` → `design` + `engineering_design`,
shipped 2026-07-30) is the reference implementation and the source of the
traps below.

## Goals / Non-Goals

**Goals:**

- A `creative` category covering video, animation, art, audio and photography.
- Named roles for those crafts, and for the game-development titles that
  resolve to a coarse category or to nothing.
- The creative toolchain in the skill dictionary, precision-first.
- The category selectable in the web picker, not merely generated into the
  contracts.

**Non-Goals:**

- No game category. `Game Developer` is software and `Game Designer` is
  design; a third category would take rows away from two working facets to buy
  a name.
- No re-cut of `design`. Motion, graphic, visual and brand design stay where
  they are; only audio moves, and it was never product design.
- No description-derived signal. As with the design split, a "Senior Designer"
  at a video studio is indistinguishable from a product designer by title
  alone, and reading descriptions is a deliberate non-goal here too.
- No `REINDEX_DEDUP` pass. Nothing in this change touches
  `role_fingerprint`, so the duplicate markers are unaffected.

## Decisions

### `creative` is a technical category

Chosen over the `engineering_design` treatment (non-technical, filterable, no
LLM budget). Media production for software companies is IT-industry work in the
same sense `design` and `product` are, and the population is small — roughly
830 open postings by title measurement against the live search API — so the
one-off enrichment spend is bounded. The alternative would also have required
adding a second exception to `cmd/prune`'s business rule, and that rule is
already carrying one.

### The vocabulary is qualified phrases, never bare craft words

`video`, `audio`, `art`, `sound` and `photo` appear in titles across every
discipline ("Audio DSP Engineer", "Art Director", "State of the Art"). Only
phrases resolve: `video editor`, `audio designer`, `concept artist`. Bare
"Audio Engineer" and "Sound Engineer" are left out for the same reason one step
up: they are broadcast, live sound and AV integration as often as they are this
craft, so labelling a field-service AV engineer a Sound Designer would be worse
than leaving the row unnamed.

### The craft aliases are declared LAST

Every media craft is also a tool or a second hat named inside someone else's
title — "Marketing Specialist (Photoshop, Illustrator)", "Graphic Designer &
Photographer", "Junior Motion Designer / Animator". Declared anywhere above
`design` or `marketing`, the block does not merely add a category: it TAKES
those rows, which is the one thing this change promised not to do. Declared at
the end of the table, a title resolves to the craft only when it names no other
discipline.

The stated cost: a "Social Media Video Editor" resolves to `marketing`. That is
the right side to err on — the posting stays findable on a facet already
correct for it, whereas a stolen marketing row is a regression.

Audio is the exception in both directions. `sound designer` and `audio
designer` must be declared ABOVE the bare `designer` alias (they contain the
word, which is the whole reason they were misfiled), and `sound design
engineer` / `audio design engineer` above the draughting block (they end in
"design engineer" and would fall through into draughting). Each collision gets
a regression test naming the title it must NOT take.

### Game roles are roles, not a category

`roletag.Derive` emits a named role independently of whether a category
resolves — `UGC Creator` already demonstrates this. So the game titles get
`game_designer`, `level_designer`, `narrative_designer`, `game_producer` and
`game_developer` while keeping whatever category they resolve to today.

### Ambiguous skills are gated, dropped, or kept from vouching — and which one
### depends on WHICH TABLE the entry sits in

`skilltag` has three tiers, and an entry lands in one of them by where it is
declared:

- **`ambiguousWords`** gates the WORD pass only. A single token whose bare form
  is ordinary English or another field's term of art goes here: `houdini` (the
  escapologist), `c4d` (the transplant-pathology biomarker). Declaring such a
  token as a PHRASE routes around the gate entirely — a phrase match is always
  strong — which is how `c4d` tagged a pathologist's posting.
- **`nonCorroboratingPhrases`** lets a phrase tag on its own but never vouch
  for a gated word beside it. The craft names go here — `video-editing`,
  `color-grading`, `storyboarding` — because each is a duty a coordinator or a
  product manager lists in passing, and as strong matches they lifted the gate
  onto `spring`, `unity` and `sketch`. This tier is consulted on the phrase
  pass only, so `storyboarding` is declared as a single-token phrase rather
  than a word.
- **Omission** for anything neither tier can save. `animation` is the CSS
  property of every frontend posting that also names React, and `nuke` is what
  a platform posting does to a cache next to Terraform; the gate is lifted by
  ANY strong skill, so gating them would tag exactly the postings they are
  wrong for. Same call the design split made for `visual design` and bare
  `after effects`.

## Risks / Trade-offs

- **A new alias steals rows from a working facet** → every alias added gets a
  collision test naming the title it must NOT take; the suite is the guard, not
  review.
- **The category is generated but unselectable** → `filterSections.ts` gets its
  group in the same commit as `labels.ts`; last time this defect was visible
  only to `svelte-check`, so the web check runs before the PR.
- **Enrichment spend is larger than measured** → the ~830 figure is an estimate
  from title-match ratios over the public search API, not an exact count. It is
  re-measured against the live `category` facet after the backfill, before
  concluding the rollout.
- **The facet is empty until the rebuild** → standard new-attribute window.
  `backfill-derive` writes the columns; the plain reindex publishes them.

## Migration Plan

1. Merge and deploy the binary (dictionary changes alone change nothing that is
   already stored).
2. `backfill-derive` — re-derives `category`, `skills` and `is_tech` in one
   keyset pass, ~3.5 h over 5.4M rows.
3. `systemctl stop freehire-reindexw.timer`, then `make reindex` (no
   `REINDEX_DEDUP`), then start the timer.
4. Read the live `category` facet for `creative` and the `role` facet for the
   new roles; compare against the estimate below.

### The pre-deploy baseline

Measured 2026-09-01 against the live public search API. Each figure is the
full-text total scaled by the share of the first 100 results whose TITLE
carries the phrase — the dictionary reads titles, so the full-text total on
its own overstates the population several-fold (`executive advisor`: 1710
full-text, 34 by title).

| title | est. open postings |
|---|---|
| video editor | 366 |
| videographer | 71 |
| video producer | 45 |
| animator | 94 |
| sound designer | 65 |
| photographer | 55 |
| environment artist | 32 |
| 3d artist | 23 |
| vfx artist | 21 |
| concept artist | 19 |
| technical artist | 16 |
| character artist | 16 |
| audio designer | 6 |
| photo editor | 4 |
| **total** | **~833** |

`illustrator` is not in the table: its title population could not be separated
from the Adobe product by this method, so it is deliberately unestimated. If
the post-backfill `creative` facet lands far above ~900, that alias is the
first place to look.

Prod facet counts the same day, for the categories this change borders:
`design` 35914, `engineering_design` 12742, `motion_designer` 489,
`graphic_designer` 2413, `ux_designer` 2377. Only `design` should move, and
only by the audio postings.

Rollback: revert the binary. The stored `category` values stay until the next
`backfill-derive`, and an unknown category in the column is inert — nothing
reads it as an enum.

## Open Questions

None. The category name, its tech placement, and the boundary with `design`
were settled with the user before this change was opened.
