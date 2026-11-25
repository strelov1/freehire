## Why

`internal/atsapply`'s deterministic resolver (`auto-apply-worker`, shipped) only answers a
custom employer question when its DOM id or label matches a narrow, curated list — today
that is Greenhouse's standardized identity fields plus one label rule (visa sponsorship). A
live smoke test (auto-apply-worker's task 7.1) found that most required custom questions on
a real posting are free text with no such match (language proficiency, "where did you hear
about this role") — every one of them parks, even where a grounded answer is plainly
derivable from the candidate's own CV/experience data. The sibling paid repository
`freehire-apply` already solved exactly this with `internal/drafting`: a single-shot LLM call
per unmapped free-text field, strictly grounded in the candidate's own data, with a
keyword-based check that always parks (never drafts) a sensitive field — salary, visa/work
authorization, or an EEO category — regardless of how confident the draft would be. This
change ports that pattern onto `internal/atsapply`'s existing pipeline.

## What Changes

- Add a `Drafter` capability to `internal/atsapply`: for a field `Resolve` would otherwise
  report unmapped (no id match, no label-keyword match, free-text kind), draft a grounded
  answer via `internal/llm` — but only when the field's label does not match a curated
  sensitive-keyword list (mirrors `freehire-apply/internal/drafting`'s `isSensitive`:
  salary/compensation, sponsor/visa/work authorization, gender/race/ethnic/veteran/disab/
  demographic/sexual orientation). A sensitive field always parks, never drafted, regardless
  of model confidence — the check runs before the model is ever called, the same way
  `internal/experience`'s provenance check gates what reaches a CV in the service rather
  than the prompt.
- The draft is grounded strictly in the candidate's own stored data — the experience bank's
  CANDIDATE-asserted atoms (`internal/experience`, `cv_import`/`stated_in_chat`/`manual`
  provenance only, mirroring the CV-write gate) and `internal/candidateprofile`'s identity
  fields — never invented. No raw CV prose is sent; the atoms are already the vetted,
  structured facts a CV bullet would draw from.
- A drafted field is still checked against the field's own offered options where the widget
  has any (a `select`/`checkbox_group` custom question), the same "never guess past what the
  widget offers" rule `Resolve`'s deterministic path already enforces — a draft is a
  free-text answer, not a license to invent an option label the platform never offered.
- LLM spend for this call is attributed to the candidate whose application it is, matching
  `internal/handler`'s `RunAgentAutofill` precedent (`h.llm.bind(ctx, userID, tagAutofill)`)
  — not the service credential every other cron worker uses. `cmd/auto-apply` currently
  cannot resolve a per-user credential at all (`internal/llmkey`'s `scope_test.go` treats
  every `cmd/` binary except `cmd/server` as owner-less background work); widening that
  allowlist is part of this change, not a side effect — see design.md's Decisions.

Explicitly **not** part of this change:
- Any change to the deterministic `Resolve`/`labelAnswerKeyFor` path from `auto-apply-worker`
  — this change only adds a fallback for what that path leaves unmapped.
- The alternative agentic architecture (`internal/autofillagent.Tools` driving a headless
  session) discussed and set aside earlier — this stays a single-shot drafting call bolted
  onto the existing deterministic pipeline, not a tool-calling loop.
- A human review/approve step before a drafted answer is used (unlike `freehire-apply`,
  which gates every application behind manual approval). `cmd/auto-apply` has no
  candidate-facing review surface today; adding one is a separate, larger change.
- Drafting for sensitive fields under any circumstance, confidence threshold, or opt-in.

## Capabilities

### New Capabilities
- `auto-apply-question-drafting`: LLM-drafted free-text answers for custom application
  questions the deterministic resolver leaves unmapped, strictly grounded in the candidate's
  own stored data, gated by a sensitive-field check that runs before drafting and always
  wins.

### Modified Capabilities
(none in `openspec/specs/` — `auto-apply-submit` was proposed by `auto-apply-worker`, which
is not archived yet, so there is no main spec to delta against. This change's behavior
change to `Resolve`'s unmapped path — try the drafter first, park only if it also cannot
answer or the field is sensitive — is captured as an ADDED requirement under
`auto-apply-question-drafting` instead, and should be reconciled into `auto-apply-submit`'s
own requirements when both changes archive.)

## Impact

- **New file(s) in `internal/atsapply`**: a `Drafter` interface + LLM-backed implementation,
  wired into `Resolve`'s (or a new orchestration step's) unmapped path.
- **`internal/llmkey`**: `scope_test.go`'s hardcoded `cmd/server`-only exemption widens to
  admit `cmd/auto-apply` as a second per-user caller — a deliberate policy change, not a
  workaround.
- **Reads `internal/experience`** (candidate-asserted atoms only) and
  `internal/candidateprofile` (already a dependency); no new tables.
- **Dependencies**: none new — `internal/llm` and `internal/experience` already exist.
- **Operational**: `cmd/auto-apply` gains a real per-attempt LLM call cost (previously zero)
  for postings with unmapped free-text questions; bounded per attempt, not per field, by
  design.md's Non-Goals.
