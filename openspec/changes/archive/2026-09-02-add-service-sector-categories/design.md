## Context

Three waves in, 766 437 open postings still reach the index with no role. Four
clusters are what is left that has a shape:

| cluster | open | groups |
|---|---|---|
| logistics | 47 551 | 922 |
| education | 23 226 | 363 |
| personal services | 6 851 | 73 |
| administration | 4 363 | 86 |

The unbucketed residue is 684 446 across 23 389 title groups, and much of it is
not classifiable by title at all: "Specialist" (1 206), "Мастер" (976),
"SUPERVISOR" (1 077), "Team Leader" (1 383) — plus postings that are not
vacancies, like "General Application" (1 135) and "Initiativbewerbung" (1 023).
This wave is the last one where a cluster is worth naming.

## Goals / Non-Goals

**Goals:**

- Four categories for the remaining service work.
- The Russian service vocabularies, none of which carries an English alias.
- The plural gap in the shipped trades vocabulary.
- The two titles the previous wave missed outright.

**Non-Goals:**

- **The 684k residue.** Generic titles and non-vacancies. A dictionary that
  reads titles cannot resolve "Специалист", and inventing a category for
  "General Application" would be worse than leaving it unroled.
- No description signal.

## Decisions

### `водитель` is the hazard this wave is built around

The wave-3 mining pass filed `Делопроизводитель` — an office clerk — under
logistics, because it *ends in* `водитель`. That was a defect in the mining
script, which matched raw substrings; the production dictionary matches on word
boundaries. But this wave adds `водитель` as a real bare alias for the first
time, and `делопроизводитель` as a real alias in a DIFFERENT category, so the
pair now exists in production for real.

The regression test asserts the clerk resolves to `administration`, not merely
that it resolves to something. A test that only checked "not empty" would have
passed while the rows were wrong.

### Administration is small, and gets its own category anyway

4 363 open is thin for a new value, and `operations` already exists. But
`operations` in this vocabulary means the back-office of an IT company — that
is what the comment beside it says — and filing a court clerk there muddies a
facet that currently works. A clean 4k category costs less than a dirty 30k
one.

### The plural gap is a defect, not an omission

`wordmatch` matches whole words and has no morphology. Nothing in an alias list
shows that a singular entry cannot reach a plural title, and the shipped trades
vocabulary was written singular throughout — so "Automotive Mechanic" resolved
and "Automotive Mechanics" did not, for 3 030 open postings across the three
largest automotive spellings in the catalogue.

The fix is to add the plural spellings, and the modified requirement states the
rule so the next vocabulary written here starts from it.

### Bare `host` joins hospitality

3 446 open postings are titled exactly "Host", and the previous wave declared
only `host/hostess` and `hostess`. The bare word collides with web hosting, but
every hosting title in this catalogue names the thing it hosts ("Hosting
Engineer", "Web Host") and resolves far above through its own discipline. The
hospitality block is declared late, so what reaches it names no other
discipline.

## Risks / Trade-offs

- **`водитель` inside `делопроизводитель`** → whole-word matching prevents it;
  a regression test pins the pair AND the two distinct categories.
- **Bare `host`** → declared in the late hospitality block, below every
  discipline that names itself; a regression test pins that a hosting title is
  unaffected.
- **Bare `coach`** → NOT added. "Agile Coach" resolves to project management
  above, but "Career Coach" and "Sales Coach" are ambiguous, and the catalogue's
  volume here is private sports coaching, which the qualified spellings reach.
- **Re-measurable** → each category should land near the figure above.

## Migration Plan

1. Merge and deploy.
2. Folded into the `backfill-derive` already scheduled on the host for 02:12
   UTC (`taxonomy-backfill-night.timer`, `BACKFILL_CONCURRENCY=3`, ~9 h). If
   this lands before it fires, one pass covers all four waves; if not, it needs
   its own pass in the next quiet window.
3. The scheduled `freehire-reindexw` picks the result up on its own cadence.
4. Read the live facet for each of the four.

### The measured result

| | open postings | share |
|---|---|---|
| unroled before this change | 815 270 | 41.4% |
| unroled after | 672 166 | 34.1% |
| **gain** | **143 104** | |

Where the newly-filled categories landed: healthcare 49 768 · retail 44 282 ·
logistics 43 919 · industrial_engineering 39 377 · skilled_trades 28 657 ·
hospitality 25 430 · education 21 372 · personal_services 6 479 ·
administration 5 428.

Two numbers moved for reasons other than the new categories, and both are the
fixes in this change: `skilled_trades` went from 18 032 to 28 657 on the plural
spellings alone, and `hospitality` from 19 436 to 25 430 on the bare `host`.
The plural gap was worth ten thousand postings — more than the whole
`personal_services` category this wave adds.

Cumulative across the four waves, from the original baseline: 935 604 →
672 166. **47.5% unroled down to 34.1%**, 263 438 open postings that now carry
a role.

### A late addition: the field-facing delivery seats

Asked mid-change about "Forward Deployed Engineer and similar". The probe found
FDE already covered — and one inconsistency worth the whole detour: `roletag`
declares the hyphenated `forward-deployed engineer` and `classify` did not, so
the ROLE fired while the category stayed empty. The same word-boundary trap as
the plurals, hiding inside a title the catalogue already counted as covered.

Fixed, along with the unresolved neighbours: Professional Services
Engineer/Consultant, Partner Engineer, Deployment Strategist, Presales
Consultant, Delivery Consultant, Integration Consultant and the bare Technical
Consultant — 1 180 open. Small, but IT-profile, which is worth more per row
here than the consumer volume.

Rollback: revert the binary. Stored categories stay until the next
`backfill-derive` and are inert.

## Open Questions

None.
