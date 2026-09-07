## Context

See proposal.md - Why for the motivating gap and the live Garner Health case.

Two independent places in today's pipeline could, in principle, catch a
work-authorization/residency mismatch, and neither does on purpose:

- `PostJobAutoApply` (`internal/api/handler/auto_apply_enqueue.go`) gates enqueueing on
  ATS support, Pro plan, CV presence, and "not already applied" only.
- `internal/candidate/hardconstraint.Evaluate(JobRequirements, CVEvidence) []Blocker`
  already compares exactly this evidence — `work-authorization` (job's
  `enrichment.visa_sponsorship` + `job.Countries` vs. the candidate's asserted-or-derived
  `CountryCode`) and `location-and-work-mode` (`job.WorkMode` + `job.Countries` vs.
  `CountryCode` + `PrefersRemote`) — but every existing caller
  (`JobMatch`, match-analysis scoring) treats its output as advisory only: per the
  package's own doc comment, a blocker "never hides or downranks a job." The assembly
  that builds `JobRequirements`/`CVEvidence` from a `db.Job` and a candidate's profile
  already lives in the same package as `PostJobAutoApply`
  (`internal/api/handler/hardconstraint_inputs.go`'s `jobBlockers` /
  `buildHardConstraintInputs`), so the enqueue path can call the same helper rather than
  reassembling the inputs.

Separately, `internal/api/atsapply`'s field resolution (`resolve.go` → `draft.go`) has no
concept of a geography/residency question at all. A required question is parked only if
its label matches the existing `sensitiveTerms` list (compensation, visa/sponsorship,
EEO/demographic) — a residency question like "Current State of Residence" matches none
of those and reaches the LLM drafter, saved from a wrong submission today only by luck
(the drafted value not matching one of the form's own dropdown options).

## Goals / Non-Goals

**Goals:**
- Refuse to enqueue an auto-apply attempt when the candidate's known evidence positively
  conflicts with the job's stated work-authorization or location/work-mode requirement.
- Stop a required, unmapped geography/residency form question from ever reaching the LLM
  drafter, independent of whether a particular dropdown happens to save it.
- Reuse `hardconstraint.Evaluate` and its existing input-assembly rather than building a
  second evidence pipeline.

**Non-Goals:**
- Changing `hardconstraint`'s own contract, its "never emit a false blocker" discipline,
  or any of its existing advisory call sites (`JobMatch`, match-analysis scoring stay
  exactly as advisory as they are today).
- Building a general-purpose eligibility/compliance engine, or covering every possible
  form of residency requirement (e.g. security-clearance citizenship, which is a
  different existing facet — `clearance-facet` — not this evaluator's concern).
- Inferring or asking the candidate to fill in new profile fields (e.g. a discrete
  "state of residence") — the gate works from evidence the profile already carries.

## Decisions

**Reuse the existing evaluator and its handler-local assembly, unchanged, as a new
caller.** Alternative considered: give `hardconstraint.Evaluate` a "strict" mode that
returns a distinguishable hard-stop signal. Rejected — the function is already pure and
its output (`[]Blocker`, each with `met`) is sufficient; whether a `false` `met` on
`work-authorization`/`location-and-work-mode` should refuse an action or only warn is a
property of the *caller*, not the evaluator. Keeping the package itself untouched avoids
any risk of the new caller's stricter interpretation leaking into the advisory ones.

**Gate on category, not on overall score-cap.** `PostJobAutoApply` checks for an unmet
blocker in exactly the `work-authorization` or `location-and-work-mode` categories,
not on the evaluator's overall score-cap ceiling (which also folds in
experience/education/language/certification — categories irrelevant to whether the
*submission itself* would misrepresent the candidate).

**Ship the enqueue refusal behind a shadow-first flag**, mirroring the existing
`PLAN_ENFORCE` precedent (`internal/ai/plan`): log what would have been refused for a
run before actually refusing it. An eligibility evaluator wired into a brand-new call
site is exactly the kind of change where a false positive (refusing a Pro candidate who
is, in fact, eligible) has an immediate, visible cost — a paying user who can no longer
auto-apply to a job they qualify for — so it earns the same rollout caution the plan
limits gate uses, rather than shipping directly enforced.

**Add a geography/residency label list to `internal/api/atsapply`, separate from
`sensitiveTerms`.** Alternative considered: extend `sensitiveTerms` itself. Rejected —
`sensitiveTerms` exists to keep the model away from a category of question on policy
grounds (compensation, demographics, visa) regardless of whether an answer is knowable;
this new list exists because the pipeline cannot *verify* an answer, which is a
different reason to park and should stay a separate, named rule so the two are not
conflated in review or in the AGENTS.md narrative. It is checked ahead of the existing
id-match → label-match → draft ordering, as a park rather than a fourth resolution tier.

## Risks / Trade-offs

- **The enqueue gate only catches what the job's enrichment already encodes
  structurally** (`visa_sponsorship`, `Countries`, `WorkMode`) → most postings, including
  the live Garner Health one, likely carry no explicit `visa_sponsorship=false`
  enrichment, so this gate alone would not have caught that specific case; the form-level
  park rule is the layer that would. Mitigation: ship both layers together — they cover
  different failure shapes (structured job data vs. a platform's own custom question),
  and neither alone is the fix this proposal is for.
- **Over-parking**: a geography-label match could park a question the pipeline already
  answers today (e.g. "City" from the candidate's known location) if the label list is
  too broad. Mitigation: the park check runs only for a question left unmapped after the
  existing id/label resolution already tried — a question with a known answer is filled
  exactly as before and never reaches this rule.
- **Two "blocker" meanings in one codebase**: `hardconstraint.Blocker` stays advisory
  everywhere else, but now also drives a hard refusal at one call site. Mitigation:
  document this explicitly in both `internal/application/autoapply/AGENTS.md` and
  `internal/candidate/hardconstraint`'s own docs, so a future reader does not assume
  "advisory only" is a package-wide invariant.

## Migration Plan

No schema or data migration. Roll out as a shadow-logged flag (see Decisions) for one
observation period, read the log for false-positive rate among Pro candidates, then flip
to enforced the same way `PLAN_ENFORCE` graduates a feature. No rollback beyond
reverting the flag — the gate makes no state changes of its own (it only refuses to
enqueue or refuses to draft).

## Open Questions

- The exact geography/residency label phrase list for the `atsapply` park rule (e.g.
  "state of residence", "country of residence", "must be based in", "current location")
  is an implementation detail to pin down against real captured `apply_forms` data during
  implementation — it does not change the requirement itself (recognize and park such a
  question) or the task breakdown.
