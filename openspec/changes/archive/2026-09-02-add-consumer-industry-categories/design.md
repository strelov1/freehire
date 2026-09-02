## Context

Two waves in, 891 574 open postings still reach the index with no role. The
engineering residue is gone; what remains is the consumer industries a broad
multi-industry ATS crawl brings in with the boards it wants.

The clusters, measured over the 60 000 largest open title groups with waves 1
and 2 applied:

| cluster | open | title groups |
|---|---|---|
| healthcare | 78 544 | 2 264 |
| skilled trades | 67 357 | 2 096 |
| retail | 47 520 | 675 |
| hospitality | 31 452 | 679 |

Healthcare is the largest and the cleanest: nearly half is Russian and entirely
uniform, two hundred spellings of `Врач-…`.

## Goals / Non-Goals

**Goals:**

- Four categories, filterable, off the enrichment budget.
- The Russian medical and trade vocabularies.
- The grocery `Clerk` family filed as retail, where it belongs.
- Named roles for the seats carried in volume.
- Deletion behaviour unchanged, provably.

**Non-Goals:**

- **Logistics, education, office administration, personal services.** Their
  clusters need their own boundary work — the mining pass put
  `Делопроизводитель` (a clerk) in logistics and `Grocery Clerk` in office
  administration — and four categories is already the most a single review can
  carry.
- **No craft protection.** These are not `engineering_design`. See below.
- No description signal.

## Decisions

### The four are NOT added to the craft set, and that is what keeps this neutral

`vocab.NonTechCraftCategories` exists so `cmd/prune` cannot delete a category
that is non-technical because the CRAFT sits outside IT. A nurse and a line
cook are not that: they are exactly the non-technical business the rule exists
to remove at a company with no technical history.

Leaving them out is also the only choice that changes nothing. Today these
postings carry no category and an unknown `is_tech`, so they match
`ruleUnknown`. Afterwards they carry a non-technical category and match
`ruleBusiness`. Both rules fire only where the company has never shown
technical evidence — same postings, same outcome, different rule name. Adding
them to the craft set would make ~225 000 postings newly *undeletable*, which
is a policy change disguised as a vocabulary change.

### `врач` is a bare token; the compounds are the hazard

Russian medical titles hyphenate (`Врач-терапевт`, `Врач-акушер-гинеколог`) or
postfix a phrase (`Врач ультразвуковой диагностики`). A hyphen is a word
boundary, so the bare token reaches all of them and no qualified alias could.

The hazard is the reverse direction: a short alias hiding INSIDE a longer word.
`Делопроизводитель` — an office clerk — ends in `водитель`, "driver".
`Электромеханик` contains `механик`. `wordmatch` matches on word boundaries and
cannot make that mistake, but nothing in the alias list shows the hazard, so
each pair gets a regression test. The mining script, which used naive substring
matching, filed `Делопроизводитель` under logistics — a live demonstration of
what the boundary rule is preventing.

### Ordering inside the four

The collisions are all "qualified spelling versus bare family word":

- `Medication Technician` is healthcare, not a trade — declared above the
  `technician` family.
- `Store Driver` and `Delivery Driver` are retail and logistics respectively,
  both above any bare driver alias (which this change does not add).
- `Kitchen Assistant` is hospitality, above any bare `assistant`.
- `Patient Coordinator` is healthcare, above any bare `coordinator`.

The general rule this repository has converged on holds: the qualified
spellings go above, the bare family word below, and each collision carries a
test naming the title it must not take.

### The grocery clerks are retail

`Deli Clerk` (1 441), `Grocery Clerk` (703), `Produce Clerk` (738), `Bakery
Clerk` (721), `Meat Clerk` (673) and their part-time spellings are shop floor,
not office administration. Filing them by the word "clerk" would put a
supermarket's whole staff in the same facet as a receptionist.

## Risks / Trade-offs

- **The public vocabulary grows by four values no IT candidate will choose** →
  accepted deliberately. A facet is as useful for excluding as for selecting,
  and 225 000 unfilterable postings are worse than four unused options.
- **A bare alias claims a compound** → whole-word matching prevents it and a
  regression test pins each known pair.
- **Deletion behaviour drifts** → a test asserts the craft set still contains
  exactly the two engineering categories, and the prune suite keeps its cases
  for both the business and unknown rules.
- **Re-measurable** → after rollout each category should land near the figure
  above; materially more means an alias took something it should not have.

## Migration Plan

1. Merge and deploy.
2. `backfill-derive` at `BACKFILL_CONCURRENCY=2-3` in a quiet window
   (02:00–05:00 UTC). At 6 it degraded the site while the worker's own log
   stayed smooth, so health is measured with `curl`, not from the log.
3. Stop `freehire-reindexw.timer`, rebuild, start the timer. No
   `REINDEX_DEDUP`.
4. Read the live facet for each of the four and compare against the table
   below.

### The measured result

| | open postings | share |
|---|---|---|
| unroled before this change | 891 574 | 45.2% |
| unroled after | 766 437 | 38.9% |
| **gain** | **125 137** | |

Split: 25 697 from the named roles alone, which fire whether or not a category
resolves, and 99 440 from the four new categories.

Where they landed:

| category | open |
|---|---|
| healthcare | 48 448 |
| retail | 44 282 |
| hospitality | 19 436 |
| skilled_trades | 18 032 |

`skilled_trades` is far below the 67 357 the mining pass sized, and the reason
is worth recording: that pass matched raw substrings over a broad keyword net
and swept in every "…Technician" in the catalogue, including "Medication
Technician" and "Pest Control Technician". The shipped vocabulary names
specific spellings. Sizing a cluster and shipping one are not the same number,
and the gap is where the guesses were declined.

Cumulative across the three waves, from the original baseline: 935 604 →
766 437, **169 167** open postings that now carry a role — 47.5% unroled down
to 38.9%.

Rollback: revert the binary. Stored categories stay until the next
`backfill-derive` and are inert. A reverted binary also restores the previous
prune behaviour, which is the same behaviour — that is what makes this safe to
roll back at any point.

## Open Questions

None.
