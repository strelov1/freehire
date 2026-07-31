# Job-match scoring conventions

## Scope
Deterministic, dictionary-only scoring of a **tailored CV against the vacancy it is bound to**.
Backend `internal/cvmatch` + `internal/handler/cv_job_match.go`; frontend in
`web/src/lib/tailor/JobMatch.svelte` and its unit-tested view model `web/src/lib/tailor/jobmatch.ts`.

## Always true
- **The document is the only subject.** The scorer never reads the base CV, the experience bank
  or the structured résumé — those describe the *candidate*. That is what distinguishes this
  score from `internal/matchanalysis`, whose numbers do not move when the CV is edited.
- **No LLM, ever.** Pure and I/O-free (`cvmatch.Compute(Input) Score`), so it costs no AI credits
  and the workspace recomputes it after every saved edit.
- **The rendered text layer is the input**, never the stored document — same rule and same helper
  (`cvHandlers.renderedCVText`) as the ATS delta, so the two can never disagree about what the
  CV says.
- **One render, not two.** The ATS delta renders base *and* tailored; this renders only the
  tailored copy. That halving is what pays for the per-save refresh cadence.
- **Four categories, server-owned weights:** Requirements Coverage 40 / Keyword Match 30 /
  Job Title Match 20 / Seniority Fit 10. The weight travels **in the response** — the client
  renders "impact" from it and must never re-declare the table.

## The one rule everything turns on

> **An unverifiable input leaves the denominator, never the numerator.**

    overall = round(Σ earned(available categories) ÷ Σ weight(available categories) × 100)

A category that cannot be evaluated is reported `available: false` with a reason and is excluded
from **both** sides. It is never scored zero — that would punish the candidate for a gap in our
dictionaries rather than in their CV. `Score.Contributing` names what the overall was taken over,
so a three-category score is never mistaken for a four-category one.

The rule recurses one level down, in two places:
- inside Requirements Coverage, an unverifiable requirement leaves that category's denominator;
- inside Job Title Match, a vacancy title the dictionary resolves to no role category leaves that
  category's denominator (its reported `Weight` drops from 20 to 12).

So `Category.Weight` is **the sum of the checks that could be evaluated**, not a constant.

## Requirements Coverage — the corroboration trap

Requirement *texts* and *priorities* come from the cached fit analysis (they describe the posting).
Their statuses do **not**: those were reached against the base profile and are recomputed here.

A requirement's skills are **the vacancy's own canonical skills that its text names** —
`skilltag.Canonicalize` over the line's 1..3-grams, filtered down to `job.Skills`.

Do **not** replace this with `skilltag.Parse(requirement.Text)`. Parse applies a corroboration
rule: an ambiguous alias (`go`, `react`, `swift`, `spring`) tags only when the same text carries
another strong technical term. That is right for a document and wrong for one line —
`"5+ years of Go"` parses to nothing, and the heaviest category would evaporate on the most
ordinary requirements. `job.Skills` were resolved from the full description where that context
existed, so attributing them is not discovery and needs no corroboration.

Consequence worth keeping: a requirement can never be attributed a skill outside `job.Skills`, so
this category and Keyword Match cannot disagree about what the vacancy asks for.

## Wire-shape gotchas
- `cvmatch.Category` is named **`ScoredCategory`**: `cmd/gen-contracts` emits into one flat
  TypeScript namespace where `Category` is already the facet enum.
- `lineitem.go` is **excluded** from codegen (see `cmd/gen-contracts`): its `LineItem`/`Status`
  are the same shape `atscheck` already emitted, and the panel renders both scorers' rows through
  one `ScoreCategoryRow`. If the two shapes ever diverge, that exclusion has to go.
- The endpoint's envelope is `{available, reason?, score?}` — an unavailable score is an absence
  the panel renders as nothing, never an error. Both a missing toolchain and a vacancy nothing
  could be matched against answer this way.
