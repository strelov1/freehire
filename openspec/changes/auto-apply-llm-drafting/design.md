## Context

See `proposal.md` for motivation. This bolts onto `auto-apply-worker` (shipped): the
deterministic pipeline in `internal/atsapply` — `ScanGreenhouseForm` → `Reconcile` →
`Resolve` — is unchanged by this document; this change only adds what happens to a field
`Resolve` would otherwise report unmapped.

Existing pieces this reuses, unchanged in behavior:
- `internal/experience.Store.ListAtoms(ctx, userID) ([]Atom, error)` /
  `.ListEmployments` — the candidate's durable experience record. `Atom.Provenance` gates
  what may be quoted (`Provenance.Publishable()`: `cv_import`/`stated_in_chat`/`manual`, not
  `agent_inferred`) — the same check `internal/cvedit`'s CV-write gate already runs, applied
  here at read time instead.
- `internal/candidateprofile.Assembler` — already `cmd/auto-apply`'s answer source (identity
  facts); this change adds no new profile data, only a new consumer of the experience bank.
- `internal/llm.Client` — the provider-agnostic wrapper every other LLM feature in this repo
  uses (schema-cache structured output, streaming, Langfuse tracing).
- `internal/llmkey.Resolver` / `Client.As` — per-user gateway credential attribution, the
  same primitives `internal/handler`'s `userLLM` composes for every other per-user feature.

Reference for the pattern being ported: `freehire-apply/internal/drafting` — deterministic
mapping first, a single-shot LLM call per free-text field, a keyword `isSensitive` check that
always wins. Read directly (sibling repo, not a Go dependency) rather than linked, since
`freehire-apply` is a separate, paid repository.

## Goals / Non-Goals

**Goals:**
- Answer the class of question the 2026-09-02 live smoke test found costs the most real
  postings: free-text custom questions with no id/label match, where a grounded answer is
  plainly derivable from the candidate's own data.
- Never weaken the "never guess" invariant `auto-apply-worker` was built around — a
  sensitive field or an ungroundable field still parks, exactly as today.

**Non-Goals:**
- A human review/approve step before a drafted answer is used. `freehire-apply` gates
  every application behind manual approval; `cmd/auto-apply` has no review surface, and
  building one is a separate, materially larger change (a UI, an approval queue state
  machine) this document does not scope.
- Multiple candidate LLM calls per attempt, or a retry-with-different-prompt strategy. One
  call per unmapped free-text field, once; a failed or empty draft parks that field, it does
  not retry the model.
- Widening `labelAnswerKeyFor`'s deterministic categories further (e.g., a smarter
  work-authorization matcher). That stays deliberately narrow per `auto-apply-worker`'s own
  design — this change adds a fallback BEHIND it, not a replacement for it.
- Sensitive-field drafting under any opt-in, confidence threshold, or override. Off means off.

## Decisions

### The sensitive-keyword check runs before the model is ever called, not after

Mirrors `freehire-apply/internal/drafting.isSensitive` and this repo's own
`internal/experience` convention (provenance decides publication, checked in the service,
never trusted to a prompt instruction). **Alternative considered**: ask the model to decline
sensitive questions itself (a system-prompt instruction). Rejected outright — an instruction
is not a gate, and the one category of mistake this change cannot afford (a drafted answer to
"do you require visa sponsorship") is exactly the one a prompt-only defense fails silently on
the day a provider or model version changes how it weighs instructions.

### Grounding source is the experience bank's publishable atoms, not raw CV text

**Alternative considered**: send the stored CV's rendered text (what
`internal/resumeextract`'s own LLM calls do, through `internal/pii`'s fail-closed redactor).
Rejected for this narrow use: a custom question's answer is a sentence or two grounded in
discrete facts (a skill, a past role, a certification), which is exactly what
`experience.Atom` already stores as vetted, structured, per-candidate evidence — sending the
whole CV would be more context for a shorter answer, and would reopen the PII-redaction
surface (`internal/pii`) for a call that does not need it, since `Atom` content is
candidate-authored short claims, not free-form prose carrying contact details. If a future
draft needs prose reasoning over the full CV, that is the point to bring `internal/pii`'s
redactor in — not before there is a concrete call site that needs it.

### `cmd/auto-apply` becomes a second per-user LLM caller, alongside `cmd/server`

`internal/llmkey`'s `scope_test.go` currently treats every `cmd/` binary except
`cmd/server` as owner-less background work and fails the build if one imports `llmkey`.
That rule is correct for `enrich`/`embed`/`telegram`/`mailclassify` — a catalogue vacancy has
no owner — but `cmd/auto-apply`'s work is a specific candidate's own application, submitted
asynchronously rather than interactively; the ownership is identical to
`RunAgentAutofill`'s, only the trigger (a queue claim, not an HTTP request) differs.
**Alternative considered**: keep the drafting call on the service credential, like every
other cron worker. Rejected — it contradicts root `AGENTS.md`'s own stated rule ("every model
call made for a signed-in user goes out on that user's OWN gateway credential") for no reason
other than the call happening to originate from a queue claim instead of a request; the
candidate is exactly as identifiable either way (`Claimed.UserID`). This change updates
`scope_test.go`'s allowlist to admit `cmd/auto-apply` explicitly, by name, with the same
one-line reasoning `cmd/server`'s existing exemption carries — not a loosened or pattern-based
rule that would admit a future background worker by accident.
- **Sub-decision**: `userLLM` (`internal/handler/user_llm.go`) — the ~15-line composition of
  `llmkey.Resolver.For` + `llm.Client.As` every per-user feature in `internal/handler` already
  shares — gets a second real caller with this change. Extract it (renamed, since
  `internal/handler` is not importable from `cmd/auto-apply` either way) into `internal/llmkey`
  itself, where both `Resolver` and the composition naturally live, rather than duplicating
  ~15 lines into `cmd/auto-apply`. A second real caller is the concrete need the "no
  abstraction before it's needed" rule asks for before extracting — this change provides it.
- **Sub-decision**: the feature tag is a new one, `feature:auto-apply-drafting`
  (`internal/handler/user_llm.go`'s existing tag constants apply per-surface — `cmd/auto-apply`
  needs its own, defined beside wherever `userLLM` lands, not borrowed from `tagAutofill`,
  since it is a materially different feature with its own cost to track).

### The drafter is a narrow interface, tested with a fake — no real model call in unit tests

Mirrors every other LLM-backed feature in this repo (`matchanalysis`, `resumeextract`):
```go
type Drafter interface {
    Draft(ctx context.Context, question MergedField, grounding GroundingContext) (string, bool, error)
}
```
`bool` is whether a groundable draft was produced at all (a model call that legitimately
found nothing to say returns `false`, not an empty string treated as an answer). The real
implementation wraps `internal/llm.Client` with a structured-output schema (label, grounded
answer or explicit "no basis"); tests exercise the sensitive-gate, the atom-provenance
filter, and the "still must match an offered option" rule against a fake `Drafter`, matching
`internal/atsapply`'s existing testing discipline for everything except live browser
interaction.

## Migration Plan

- No schema change, no migration.
- `scope_test.go`'s allowlist edit is the one change with a blast radius beyond
  `internal/atsapply`/`cmd/auto-apply` — reviewed as its own commit, since it changes a rule
  every other background worker in the fleet is held to.
- `cmd/auto-apply` gains its first real LLM-call cost. Bounded per-attempt (one call per
  unmapped free-text field a sensitivity check passes, not a retry loop), but this is still a
  new, nonzero operational cost on a worker that cost nothing before — worth a note in
  `internal/atsapply/AGENTS.md` once built, the way `internal/config.LoadAutoApply`'s doc
  comment already flags this worker's other real-side-effect costs.

## Risks / Trade-offs

- **[Risk]** A sensitive-keyword list is inherently incomplete — a novel phrasing of a
  work-authorization question that doesn't contain any listed keyword slips past the gate →
  **Mitigation**: the list starts as a direct port of `freehire-apply`'s own
  `isSensitive` terms (already measured against real Ashby postings there), not invented
  fresh; widening it as new phrasings are found is expected maintenance, not a design flaw to
  solve now.
- **[Risk]** `experience.Atom`'s publishable content could still indirectly carry something
  that reads as sensitive (a past employer's name that itself signals a work-authorization
  fact, say) even when the question label passes the keyword gate → **Mitigation**: none
  designed here; the gate is on the QUESTION, matching `freehire-apply`'s own scope — carried
  forward as an open risk, not solved by this change.
- **[Trade-off]** Grounding from atoms only, not full CV prose, means a draft can be thinner
  than a human-written answer would be for a question that needs narrative reasoning across
  several roles → accepted; the alternative (full-CV grounding) reopens the PII-redaction
  surface for a call this change's scope does not need it for (see Decisions).
