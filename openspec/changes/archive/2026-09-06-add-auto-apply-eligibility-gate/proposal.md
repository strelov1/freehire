## Why

Nothing in the auto-apply pipeline checks whether a candidate is actually eligible —
on work-authorization or residency grounds — for the job it is about to apply to on
their behalf. `PostJobAutoApply` enqueues on ATS support, Pro plan, CV presence, and
"not already applied" alone; once queued, a required residency/authorization question
the platform's own sensitive-keyword list does not happen to recognize is handed to the
LLM drafter, which will attempt an answer rather than parking. This was found live: a
Garner Health Greenhouse posting required "Current State of Residence" (a US-states-only
dropdown) for a candidate based in Brazil, not authorized to work in the US and not
willing to relocate — the only thing that stopped a submission was that no US state
happened to match the candidate's address, an accident of that one form, not a decision
this system made on purpose. The codebase already has a deterministic, dict-only
evaluator for exactly this comparison (`internal/candidate/hardconstraint`,
`hard-constraint-matching`), but it is wired only into advisory match-score displays,
never into anything that refuses to act.

## What Changes

- `PostJobAutoApply` runs `hardconstraint.Evaluate` for the (candidate, job) pair before
  enqueueing and refuses to enqueue when it reports an unmet `work-authorization` or
  `location-and-work-mode` blocker, with a response the caller can render as a reason
  rather than a generic failure.
- `internal/api/atsapply`'s field resolution treats a required, unmapped question whose
  label matches a geography/residency pattern (state/country/residence, distinct from the
  existing visa/sponsorship sensitive terms) as a designed park reason, evaluated before
  the LLM drafter ever sees it — not left to depend on the drafted value accidentally
  failing to match a dropdown option.
- No change to `hardconstraint`'s own contract or its existing advisory call sites
  (`JobMatch`, match-analysis scoring): those keep never hiding or downranking a job.
  This adds a new caller with different stakes — an unattended submission has no human to
  override an advisory warning — not a new meaning for the existing one.

## Capabilities

### New Capabilities
- `auto-apply-eligibility-gate`: refuses to enqueue an auto-apply attempt, and refuses to
  let a required unmapped geography/residency question reach the LLM drafter, whenever
  the candidate's known work-authorization or location-and-work-mode evidence conflicts
  with what the job requires.

### Modified Capabilities
(none — `hard-constraint-matching`'s own evaluator and existing advisory call sites are
unchanged; this adds a new caller of `hardconstraint.Evaluate`, not a new requirement on
it)

## Impact

- `internal/api/handler/auto_apply_enqueue.go` (`PostJobAutoApply`): new gate, new refusal
  response shape.
- `internal/candidate/hardconstraint`: new caller (`buildHardConstraintInputs`-equivalent
  assembly reused or adapted for the enqueue path); package itself unchanged.
- `internal/api/atsapply`: `resolve.go` / `draft.go` / `sensitive.go` gain a geography
  park rule ahead of drafting.
- `internal/application/autoapply` (`AGENTS.md`) and `internal/api/atsapply`
  (`AGENTS.md`): documentation of the new gate and its rationale.
- No schema changes expected — `job.Enrichment.VisaSponsorship`, `job.Countries`,
  `job.WorkMode`, and the candidate's `location_preferences`/CV-derived country already
  exist and already feed `hardconstraint.Evaluate` elsewhere.
