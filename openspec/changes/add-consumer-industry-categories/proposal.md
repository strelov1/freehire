## Why

After the IT and industrial waves, **891 574 open postings still reach the
search index with no role**, and the residue is no longer engineering of any
kind. It is the consumer industries a broad multi-industry ATS crawl brings in
with the boards it wants:

| cluster | open postings |
|---|---|
| healthcare | 78 544 |
| skilled trades | 67 357 |
| retail | 47 520 |
| hospitality | 31 452 |

Nearly half of healthcare is Russian and completely uniform — `Врач-терапевт`
(1 152), `Врач-невролог` (1 140), `Врач-хирург` (1 021), `Ветеринарный врач`
(1 080) and two hundred more spellings of `Врач-…`, none of which resolves to
anything today.

These postings are already in the catalogue and already served; they are simply
unfilterable. A candidate cannot exclude them and a recruiter cannot find them.

**The trade-off is worth stating plainly:** this is an IT job aggregator, and
these categories name work it does not exist to serve. Making them filterable
is the point — a facet is as useful for excluding as for selecting — but it
does grow the public vocabulary by four values that no IT candidate will ever
choose.

## What Changes

- Add four categories: `healthcare`, `skilled_trades`, `retail`,
  `hospitality`.
- All four are **non-technical** and, unlike `engineering_design` and
  `industrial_engineering`, are NOT added to the craft set that `cmd/prune`
  spares. That keeps today's deletion behaviour exactly: these postings
  currently match `ruleUnknown` (no category, `is_tech` unknown, no company
  evidence); afterwards they match `ruleBusiness` on the same condition. Both
  rules require the company to have no technical history, so nothing becomes
  newly deletable.
- Add the Russian medical vocabulary (`врач`, `медсестра`, `фельдшер`,
  `санитар`), the Russian trades (`электрик`, `сварщик`, `слесарь`,
  `плотник`, `механик`) and the English families around them.
- Resolve the grocery `Clerk` family to `retail` rather than to office
  administration: "Deli Clerk", "Grocery Clerk" and "Produce Clerk" are shop
  floor.
- Add named roles for the seats the catalogue carries in volume.

Logistics, education, office administration and personal services are a
separate change: the mining pass shows their clusters need their own
boundary work (`Делопроизводитель` is a clerk, not a driver), and four
categories is already the most that can be reviewed at once.

## Capabilities

### New Capabilities
- `consumer-industry-taxonomy`: which titles resolve to `healthcare`,
  `skilled_trades`, `retail` and `hospitality`, the Russian vocabularies inside
  them, why they are non-technical WITHOUT being craft-protected from prune,
  and the named roles they expose.

### Modified Capabilities
<!-- None. `catalog-pruning`'s craft-set requirement is unchanged: these four
     categories are deliberately NOT members, and the requirement already
     states the set is what the rule subtracts. -->

## Impact

- `internal/dict/vocab` — `CategoryValues` and `NonTechCategories`. NOT
  `NonTechCraftCategories`.
- `internal/dict/classify` — the title `categoryTable`; ordering decides every
  collision between the four.
- `internal/dict/roletag` — `categoryNoun` for all four (required by the
  bare/graded role grid) plus the named roles.
- `cmd/gen-contracts` output, `web/src/lib/labels.ts`,
  `web/src/lib/filterSections.ts` (which needs a new group — none of the
  existing eight fits a nurse) and `extension/lib/labels.ts`.
- Rollout: `backfill-derive` at `BACKFILL_CONCURRENCY=2-3` in a quiet window,
  then a plain `make reindex`.
- Cost: none. All four are non-technical, so they stay off the enrichment and
  embedding queues.
