## Context

`Drafter` (`internal/api/atsapply/draft.go`) is one interface, one method:
`Draft(ctx, question MergedField, grounding GroundingContext) (answer string, ok bool,
err error)`. `ResolveWithDrafting` offers it every `draftable` field — `required`, labeled,
`Kind` in `{text, textarea, select}`, not sensitive, not geography — regardless of kind;
the kind distinction lives entirely in how `LLMDrafter.Draft` prompts and in what
`matchOption` does with the answer afterward (passthrough for `text`/`textarea`, a
case-insensitive label match back to the platform's own `Value` for `select`).

A `select` field's options are `applyform.Option{Label, Value}` — the candidate reads
`Label`, the platform expects `Value` back, and `matchOption` already does that
translation for ANY answer, deterministic or drafted. This means a `select` field's answer
contract is already "one of the offered labels, verbatim, or nothing" — identical to what
`autofill-choose-jev-provider` validated for `autofillagent.Choose`.

A spike (2026-10-09) confirmed this against the live Typesafe API: 6/6 synthetic cases
matched, including two adversarial ones targeting `draftSystemPrompt`'s categorical
exclusion rule ("never answer identity/demographics/compensation/legal work status even
if the question seems answerable") — a stated salary-negotiation fact against a
compensation question, and a stated US on-site work history against a legal-authorization
question, both correctly declined despite a superficially supportive fact being present.
That is a stronger claim than "nothing supports any option" (which the prior change's
spike validated) and was deliberately spiked first as the riskiest unknown.

## Goals / Non-Goals

**Goals:**
- Answer a `select`-kind draftable field with Jev when `TYPESAFE_API_KEY` is configured,
  preserving `Drafter`'s exact contract: an offered option's `Label` verbatim, or `ok=false`.
- Preserve the categorical exclusion for identity/demographics/compensation/legal-status
  questions even for `select` fields answered by Jev — not just "nothing stated supports
  it", but "this topic is never answered regardless of what's stated".
- Fall back to `LLMDrafter.Draft` on any Jev-side failure, same safety net as the Choose
  change.
- Zero behavior change for `text`/`textarea` fields, and zero behavior change anywhere
  when `TYPESAFE_API_KEY` is unset.

**Non-Goals:**
- Touching `text`/`textarea` drafting, `isSensitiveLabel`/`isGeographyLabel` gating,
  `matchOption`, the cover-letter-reuse path (`answerFor`'s cover-letter branch only ever
  applies to `text`/`textarea` fields carrying `isCoverLetterTextField`, so it is
  structurally unreachable for `select` and untouched either way).
- A shared Jev client package. Each integration point gets its own small, unexported
  client — the same call this change's predecessor made, for the same reason: Jev's HTTP
  contract is one endpoint, and the two integration points (`autofillagent`, `atsapply`)
  are in different architectural layers (`ai` vs. `api`) that must not import each other
  for a thin HTTP client neither can share without creating a cross-layer dependency
  `internal/platform/arch/layering` would reject.
- `cmd/enrich`, `LLMPlanner.Plan`, anything already out of scope for the prior change.

## Decisions

**`JevSelectDrafter` wraps `LLMDrafter` rather than replacing it**, mirroring
`JevPlanner`'s shape exactly: `Draft` checks `question.Kind`, delegates `text`/`textarea`
straight to the wrapped `Drafter`, and only intercepts `select`. This keeps the free-text
generation path — never spiked, never validated for Jev — completely untouched by
construction, not by a runtime branch inside a single combined prompt.

**The categorical-exclusion instruction travels with every `select` Jev call, not just
some.** Rather than trying to detect "is this field sensitive" again (that gate already
exists, upstream, in `draftable`'s `isSensitiveLabel`/`isGeographyLabel` checks, which
exclude a field from drafting ENTIRELY), the Jev instructions always carry the same
categorical rule `draftSystemPrompt` states for the LLM path: identity, demographics,
compensation and legal work status are never answered regardless of what the grounding
atoms say. A `select` field reaching this drafter has already passed the sensitive-label
gate, so this rule is a second, independent safeguard against a label heuristic missing a
real compensation/eligibility question phrased in an unexpected way — exactly the
adversarial case the spike targeted.

**Decline is one explicit criteria key, covering both reasons at once** ("nothing stated
supports any option" OR "categorically excluded topic"), not two separate signals. A
caller (`matchOption`, then `ResolveWithDrafting`) only ever needs to know "answered or
not" — it has no use for which of the two reasons applied, so a single sentinel keeps the
response shape identical to the Choose change's validated design.

**No shared Jev client between `autofillagent` and `atsapply`.** Considered extracting
`internal/ai/autofillagent/jev.go`'s HTTP client into a shared helper. Rejected: `atsapply`
sits in the `api` block, `autofillagent` sits in `ai` — two blocks at different layers in
`internal/platform/arch/layering`'s table, and `ai` may not import from `api` nor the
reverse for a utility this thin. A ~60-line hand-rolled client duplicated twice, each
scoped to its own package, costs less than a shared package would cost in cross-layer
coupling for two call sites that may evolve independently (different instructions,
different criteria shapes already, per the designs above).

**Config: reuse `TYPESAFE_API_KEY`, no new setting.** Both integration points answer the
same question ("is Jev available in this deployment") from the same env var; nothing here
scopes it per-feature.

## Risks / Trade-offs

- **[Risk]** A `select` field whose label the sensitive/geography gates miss, combined
  with Jev misjudging the categorical-exclusion instruction on a case the spike didn't
  cover, could leak a compensation/demographic answer into a submitted form. →
  **Mitigation**: `matchOption` still re-validates any answer against the field's own
  offered labels before it is written to the plan — an answer outside the options parks
  the field, same as today; the categorical instruction is a second, independent layer on
  top of the existing label gate, not a replacement for it.
- **[Risk]** Jev reachability/latency on the auto-apply critical path, same shape as the
  prior change. → **Mitigation**: same answer — one attempt, no retries, bounded timeout,
  `LLMDrafter` fallback on any failure.
- **[Risk]** Jev's real-world accuracy on live `select` fields (longer or more ambiguous
  option sets than the 6 spiked cases) may differ. → **Mitigation**: accepted trade-off,
  consistent with the prior change — a follow-up adds comparison instrumentation if live
  mismatches surface.

## Migration Plan

- No data or schema changes. Deploy inert by default (`TYPESAFE_API_KEY` already optional
  from the prior change); rollback is unsetting the env var or reverting the one
  construction-site change in `internal/api/atsapply/client.go`.

## Open Questions

- None blocking implementation.
