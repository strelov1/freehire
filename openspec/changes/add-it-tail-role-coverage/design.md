## Context

`internal/dict/classify` reads the TITLE only and never guesses. Its
`categoryTable` resolves in DECLARATION ORDER — the first alias occurring as a
whole word wins — so placement is the design. (`roletag` is the one that matches
longest-alias-first; the two tables do not share a rule.)

The measurement this change acts on, taken against prod on 2026-09-02 by
dumping the 60 000 largest OPEN title groups with their stored category and
seniority and running the real `roletag.Derive` over them:

| | |
|---|---|
| open postings covered by the dump | 1 971 625 |
| of those, reaching the index with NO role | 935 604 (47.5%) |
| of those, whose stored category is empty | 935 604 (100%) |
| recognisably IT-shaped | 20 492 across 836 title groups |

The 100% line is the finding that shapes everything: there is no posting where
the category resolved and the role did not. `roletag` derives a bare role from
the category, so category coverage IS role coverage, and every hour spent on
role aliases while the category is empty buys nothing.

## Goals / Non-Goals

**Goals:**

- The Russian software and administration vocabulary.
- The `Systems Engineer` family, without sweeping its industrial namesakes in.
- The vendor-platform titles that state a discipline by naming a product.
- The infrastructure and end-user-IT tail.
- The named roles those titles deserve, plus the singular-spelling fix to
  `systems_administrator`.

**Non-Goals:**

- **No new category.** Everything here lands in a category that already exists,
  which is what keeps this change free of the three couplings a new category
  drags in (`roletag.categoryNoun`, the `skillvec` registry, the two apps'
  label maps).
- **The ambiguous middle is deferred, not forgotten.** `Automation Engineer`
  (677), bare `Application Engineer` (444), the SAP and Dynamics functional
  consultants (~850) and `CRM Specialist` (59) all read at least as often as
  industrial or business roles in this catalogue. They belong to the industrial
  wave, which gives them somewhere to go; guessing now would put them in
  software and be wrong for half of them.
- No description signal. Same non-goal the design split made.

## Decisions

### The non-IT lookalikes are declared BLIND, not categorised

`Control Systems Engineer`, `Power Systems Engineer`, `Electrical Systems
Engineer` and `Quality Systems Engineer` all contain "systems engineer". Since
the bare alias must exist (1440 postings spell it exactly), those four have to
be declared above it or they get swept into software.

They are declared with `categoryNone`, the sentinel a blind alias carries —
they keep resolving to nothing, exactly as today. The alternative, filing them
under some existing category, would be a guess: they are industrial engineering,
and this change does not introduce that taxonomy. The sentinel is the mechanism
the design split built for precisely this shape ("Software Design Engineer" is
software, not design) and it is documented in `design-taxonomy`.

### Bare `systems engineer` resolves to `software_engineering`

`software_engineering` is the catalogue's declared bucket for "confirmed
software/IT work naming no sub-discipline", which is what a bare "Systems
Engineer" is once the four industrial spellings above it have taken their rows.
It is not `devops`: the population is mixed between infrastructure and
generalist software work, and the generic bucket is the honest answer where
`devops` would be a guess.

### `программист` and `разработчик` resolve as BARE tokens

Unlike English, the qualified Russian spellings put the technology first —
"Java-разработчик", "Python-разработчик", "Инженер-программист". A hyphen is a
word boundary, so a qualified alias cannot cover the bare form, and the bare
form covers all of them. This is the same reasoning the existing `1c` entry
records.

### `IT Specialist` / `IT Technician` resolve to `support`, not `devops`

"IT Support Specialist" already resolves to `support`, and these two name the
same end-user IT desk. Splitting one job across two facets on the strength of
one dropped word is the defect the design split existed to fix.

### The `systems_administrator` alias fix is a bug, not an addition

The role exists and the catalogue lists it as covered, but only "Systems
Administrator" reaches it. "System Administrator", "Sysadmin" and the Linux and
Windows spellings all resolve to `devops` and then emit no named role. A role
that is present in the catalogue and unreachable from the commonest spelling of
its own title is worse than a missing one: nothing in the role picker suggests
the gap.

## Risks / Trade-offs

- **An alias steals a row from another discipline** → every alias gets a
  collision test naming the title it must NOT take. The mining pass surfaced
  the real shapes to guard: "Parts Counterperson", "Parts Interpreter", "Pit
  Technician", "SAP Operations Clerk". (Those four are artefacts of naive
  substring matching in the mining script — "count**erp**erson" contains "erp",
  "P**it T**echnician" contains "it t". The production dictionary matches on
  word boundaries and cannot make that mistake, but the tests pin it anyway,
  because the next reader will not know that.)
- **Bare `systems engineer` is genuinely mixed** → mitigated by the four blind
  spellings above it, and re-measurable after rollout: if `software_engineering`
  grows by materially more than the ~2.4k the Systems Engineer family accounts
  for, the alias took something it should not have.
- **Enrichment spend** → the newly-categorised postings become `is_tech = true`
  and enter the queue. Bounded by 20 492, and smaller in practice since this
  change takes only the unambiguous part.

## Migration Plan

1. Merge and deploy.
2. `backfill-derive` with `BACKFILL_CONCURRENCY=2-3`, in a quiet window
   (02:00–05:00 UTC). At 6 it degraded the site: load 15, the root page timing
   out, search at 26s. The worker's own log stayed smooth throughout, so health
   has to be measured with `curl`, not from the log.
3. Stop `freehire-reindexw.timer`, run the rebuild, start the timer. No
   `REINDEX_DEDUP`: nothing here touches `role_fingerprint`.
4. Re-measure: the unroled share should fall from 47.5% by roughly the volume
   this change claims, and `software_engineering` should grow by about the
   Systems Engineer family's size and no more.

### The measured gain, and the trap in measuring it

Re-running the mining pass over the same dump with the new dictionary:

| | open postings | share |
|---|---|---|
| unroled before | 935 604 | 47.5% |
| after the named roles alone | 930 122 | 47.2% |
| after the new categories too | 925 249 | 46.9% |
| **gain** | **10 355** | |

Half the gain comes from `roletag` alone: a named role is emitted whether or not
a category resolves, so "Systems Engineer" and "Системный администратор" become
filterable even before the category does.

The trap: the first attempt at this measurement re-derived the category from the
TITLE for every row and reported a gain of **minus 7 320**. The stored category
is derived from the title *and* the description, and this change touches only
the title path — so simulating it has to FILL an empty category and never
overwrite one the description supplied. A title-only re-derivation silently
discards every category the description had contributed, which reads exactly
like a regression the change did not cause.

Rollback: revert the binary. Stored categories stay until the next
`backfill-derive`, and they are inert — every consumer reads the vocabulary
from Go.

## Open Questions

None. The ambiguous titles are explicitly deferred rather than left undecided.
