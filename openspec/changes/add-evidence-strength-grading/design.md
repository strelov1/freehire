## Context

`internal/matchanalysis` runs a fixed three-stage prompt-chain. Stage 1 emits a `Requirement{Text, Priority, Status, Evidence}` table with `Status ∈ {covered, synonym-only, missing-have, missing-gap}` and a free-text `Evidence` pointer to where the CV supports it (or why it is absent). The status captures *whether* the CV covers a requirement but not *how strongly* — a listed keyword and a metric-backed accomplishment both read as `covered`. Stage 3 (the skeptic) already demotes `synonym-only` on required requirements; it has no signal to demote a thin `covered`.

The wire `Analysis` (with `RequirementMatch []Requirement`) is generated to TS via `cmd/gen-contracts` and cached per `(user, job)` triple-stamped by CV upload / job `content_hash` / model. GET never calls the LLM.

## Goals / Non-Goals

**Goals:**
- Grade the strength of the cited evidence for the two positive statuses so the served verdict and the Stage-3 audit can distinguish real ownership from a bare keyword.
- Keep the grade deterministic-to-serve and self-healing: additive field on the existing cached payload, no migration, no reindex, no new endpoint.
- Preserve the "never persist an out-of-vocabulary value" sanitize invariant.

**Non-Goals:**
- No change to `overall_score` weights or the six dimensions — strength informs the *model's* Stage-3 scoring via the prompt, it is not a new server-side weighted input.
- No deterministic (non-LLM) derivation of evidence strength — it is a Stage-1 model judgement over the CV, sanitized on the way out (mirrors how `status` works today).
- No backfill of existing cached analyses — they refresh on the next recompute.

## Decisions

- **Vocabulary (ordered, four values):** `metric` > `scope` > `responsibility` > `keyword`. A closed set kept next to the existing `Status`/`Priority` constants in `matchanalysis.go`, with a `validEvidenceStrength` map paralleling `validStatus`.
- **Field placement:** add `EvidenceStrength string json:"evidence_strength"` to `Requirement`. Positive statuses carry one of the four values; `missing-have`/`missing-gap` carry `""`. This keeps the field meaningful only where evidence exists, so the UI and audit never read a strength for an absent skill.
- **Sanitize rule:** in `sanitizeRequirements`, after coercing `Status`: if the status is positive, lower-case the strength and coerce anything not in the vocabulary (including empty) to `keyword`; if the status is a `missing-*`, force `""`. Same drop-don't-relabel discipline already used for status.
- **Stage-1 prompt:** extend the requirement instruction in `analyzer.go` to ask for `evidence_strength` on covered/synonym-only items, defined by the same four tiers, grounded in the `evidence` it already cites. One added sentence + the field in the JSON shape description.
- **Stage-3 prompt:** the `synonym-only` demotion sentence (just added) is broadened: for `required` requirements, both a `synonym-only` match and a `covered` match graded `keyword` are adjacent/thin support and must not alone sustain a high `skills_coverage`. Stage 3 already receives the requirement rows via `writeRequirements`, so the grade must be rendered there.
- **`writeRequirements` rendering:** append the strength to the existing `- [priority/status] text` line as `- [priority/status/strength] text` for positive statuses, so both the Stage-2 and Stage-3 prompts see it without a new writer.
- **Frontend:** the requirement row in `web/src/routes/match/[slug]/` renders a small strength cue (e.g. a dot/label) for positive statuses only, reading the generated `evidence_strength`. No layout rework.

## Risks / Trade-offs

- **Model may over-grade to `metric`.** Mitigated by (a) grounding the grade in the `evidence` string it must already cite, and (b) Stage 3 auditing inflated dimensions — a `metric` claim unsupported by the CV is exactly what the skeptic prunes. The grade is advisory to the model, never a server-side score multiplier, so a wrong grade cannot mechanically inflate `overall_score`.
- **Added prompt tokens.** One field per requirement row (≤30 rows) and two prompt sentences — negligible against the CV + job body already in-context.
- **Stale cached analyses lack the field.** They deserialize to `""` and render as `keyword`/no-cue until the next recompute; acceptable since recompute is already offered on any stamp change and the field is additive.
