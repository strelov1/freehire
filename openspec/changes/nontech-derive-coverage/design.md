## Context

See `proposal.md` — Why, for the motivating numbers. Full investigation and
rejected alternatives: `docs/superpowers/specs/2026-09-19-nontech-derive-coverage-design.md`
(design record, status "design approved, not implemented" until this change
lands — update its status line once archived).

The three dictionaries this touches are dict-only and never guess
(`internal/dict/classify/AGENTS.md`, `internal/dict/skilltag/AGENTS.md`,
`internal/dict/location/AGENTS.md`). `search.CategoryUnresolved` excludes any
job whose `category` is empty and `is_tech` is not confidently true — so an
unresolved title in this segment is not merely unfacetted, it is absent from
Meilisearch entirely. That is why the category work leads and everything else
follows it.

## Goals / Non-Goals

**Goals:**
- Resolve the specific title/skill/work-mode-phrase gaps the proposal
  measured, so the segment's postings re-enter the index.
- Keep every dictionary dict-only: no new alias fires without a live-title or
  live-description sample behind it.

**Non-Goals:**
- Decomposing `assistant` as a general class (rejected in the design record —
  a taxonomy project, not an answer to this segment).
- Routing non-tech postings through LLM enrichment (contradicts
  `vocab.go`'s "back-office roles are not enriched" decision).
- Country/residency eligibility matching — a separate, larger problem.
- `personal assistant` — deferred; its resolution already splits across four
  categories and needs its own live-title sample before a decision.

## Decisions

**No bare `assistant` alias.** The word states a grade ("Assistant
Controller") or qualifies a non-administrative trade ("Maintenance
Assistant", "Clinic Assistant") far more often than it names admin work.
Only qualified phrases (`admin assistant`, `virtual assistant`, …) are added.
This is what keeps the change small: it needs no qualifier entries and no
`categoryNone` sentinels for the grade-word tail, because that tail never
matches in the first place. Same class of exclusion classify already applies
to bare "safe" and bare "compliance".

**Immigration family resolves to `legal`, not `administration` or
`management`.** "Immigration Case Manager" would otherwise fall through to
the terminal `manager` → `management` rule; it must be placed ahead of that
fall-through in the alias table.

**New skill canonicals ship slug + label + description in one commit.** The
`descriptions.tsv` ratchet that once allowed an undescribed backlog is gone;
a canonical with no description fails the build now, so there is no
"add now, describe later" path available even if this change wanted one.

**`work from home` is a `travelPerkPhrases` guard candidate, not a plain
addition.** The phrase also appears in benefit-list prose ("occasional work
from home days"), the same shape that made `work from anywhere` need a guard
(freehire#2696). Deciding the guard boundary requires a sample of live
descriptions carrying the phrase, not an assumption — see Migration Plan.

**Alternatives considered:** both rejected alternatives (assistant-as-a-class,
LLM routing) are recorded above under Non-Goals with their reasons, per the
design record.

## Risks / Trade-offs

[A qualifier entry drifts wide and reclaims trap-list titles into
`administration`] → Verification step 3 (below) spot-checks the trap list
(`maintenance assistant`, `assistant controller`, `clinic assistant`,
`shipping clerk`, `surgery scheduler`) after the dictionary change and again
after backfill; this is the check that fails loudest if the bare-alias rule
is ever broken later.

[`work from home` resolves `remote` inside a travel/PTO perk sentence,
producing a false remote flag] → Sample live descriptions containing the
phrase before deciding whether it needs the `travelPerkPhrases` guard (same
process `work from anywhere` went through); do not add it unguarded on
assumption.

[The unavoidable `cmd/backfill-derive` run (~15h) is also carrying unrelated
debt] → Already true independent of this change: `classify/AGENTS.md`
records ~6,300 postings from #2847/#2849 reading `is_tech = true` against
current dictionaries. This backfill run clears that debt too; delete that
AGENTS.md section once the run completes, and don't attribute its numbers to
this change's own verification counts.

[Stacking `make reindex` behind an in-flight `search-drain` or
`reindex-companies`] → Follow the existing scheduling rule
(`internal/search/AGENTS.md`): Meilisearch runs one serial task queue, so
this reindex must not be launched on top of another rebuild.

## Migration Plan

1. Land the dictionary/skill/work-mode-phrase changes (this change's tasks).
2. Before enabling the `travelPerkPhrases` guard for `work from home`, pull a
   sample of live descriptions containing the phrase and classify each as
   work-arrangement statement vs. benefit-list mention; decide the guard
   boundary from that sample (mirrors how `work from anywhere` was decided).
3. Run `cmd/backfill-derive` (hold `BACKFILL_CONCURRENCY` at 2-3 — measured
   degrading prod at 6; ~15h) over the full catalogue.
4. Follow with a full `make reindex` — there is no incremental path, since
   none of `category`/`is_tech`/`work_mode`/`skills` are part of
   `content_hash`.
5. Re-run the three verification queries from the design record (segment
   postings excluded by `search.CategoryUnresolved`; segment postings now
   flagged `remote`; trap-list spot-check) and compare against the baseline
   numbers already recorded there.
6. Delete the "Owed right now" section of `classify/AGENTS.md` once the
   backfill completes, since this run clears that debt as a side effect.

Rollback: every alias/canonical/phrase addition is additive — no existing
alias is removed or re-pointed (per proposal.md, "Not breaking"). Reverting
the code change and re-running backfill + reindex is sufficient; there is no
data migration to unwind.

## Open Questions

None — the one genuine unknown (the `travelPerkPhrases` guard boundary for
"work from home") is resolved by sampling before rollout, per Migration Plan
step 2, rather than left open past this design.
