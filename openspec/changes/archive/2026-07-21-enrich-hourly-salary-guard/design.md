## Context

Salary is not a DB column — it lives in the `jobs.enrichment` JSONB, extracted by
the enrichment LLM (`cmd/enrich`). The contract types `SalaryMin`/`SalaryMax` as
`*int` (`internal/enrich/enrichment.go`), and the system prompt asks for
`salary_min (int)` (`internal/enrich/langchain.go`). Faced with a fractional
hourly rate under an integer contract, the model drops the decimal point
(`26.08 → 2608`), inflating the displayed rate ×100. The frontend renders the
stored value verbatim (`web/src/lib/enrichment.ts`), so the corruption is at the
source, not the view.

A live spike (privatclaw/light, the production model) confirmed the mechanism and
compared three fixes; results in "Decisions".

## Goals / Non-Goals

**Goals:**
- The LLM populates `salary_min`/`salary_max` faithfully for fractional hourly
  rates (round to whole currency units; never strip the decimal point).
- Already-corrupted jobs are re-enriched through the corrected prompt.

**Non-Goals:**
- No change to the salary representation (stays `*int`, whole currency units).
- No sub-dollar precision (cents) in storage or display.
- No post-hoc heuristic that tries to "repair" already-stored corrupted numbers.

## Decisions

**Decision: prompt guard with a concrete counter-example (variant A).**
Add one instruction to the system prompt anchoring on the exact failure:
`$26.08/hr → 26, never 2608`. The spike proved this works deterministically on
the budget model (3/3 runs returned `salary_min=26, salary_max=38, period=hour`;
the guard also corrected the `salary_period`, which the baseline mislabelled
`day`/`month`). A concrete numeric counter-example, not an abstract rule, is what
made the weak model comply.

Alternatives considered and rejected, both by spike evidence:
- **B — sanitize heuristic (divide-by-100 when `period=hour` and value large).**
  INVALIDATED. The corruption also mangles `salary_period`, so a `period=hour`
  gate misses part of the corrupted data; and any magnitude threshold also
  divides genuine high hourly rates ($1200/hr → $12/hr). The lost decimal is
  unrecoverable information.
- **C — store minor units (cents).** Faithful representation, but does not fix
  extraction on its own — the model must still emit the right number, so C would
  merely store the corruption precisely. Complementary, not a substitute;
  deferred as a noted seam if cent-precision display is ever wanted.

**Decision: bump `enrich.Version` to re-enrich corrupted rows.**
Re-enrichment is the established mechanism (`enrich.Version` is the provenance
stamp; the outbox re-enqueues jobs below the current version). This reaches every
already-corrupted posting without a bespoke backfill.

## Risks / Trade-offs

- [Weak model may still occasionally slip on an unusual pay format] → The guard is
  best-effort like all enrichment; sanitize already drops non-positive and
  min>max values. This change strictly reduces the error rate, and the spike
  showed 3/3 correct on the real posting.
- [Re-enriching the catalogue costs LLM budget] → Bounded by the existing
  freshest-first, open-jobs-only enrichment drain; the version bump is the
  standard cost of any contract correction and re-enriches incrementally.

## Migration Plan

1. Merge the prompt guard + `enrich.Version` bump.
2. Deploy `cmd/enrich`; the outbox re-enqueues jobs below the new version and
   drains them freshest-first through the corrected prompt.
3. Rollback: revert the version bump (jobs stay at the prior version; no schema
   change to undo).
