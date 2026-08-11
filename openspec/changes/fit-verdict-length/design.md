## Context

See proposal.md for motivation. Today `maxRecommendRunes = 400` is applied in
`sanitizeVerdict` after Stage 2 and again after Stage 3; Stage 3's user prompt marshals the
already-sanitized Stage-2 verdict. The verdict card in `MatchAnalysisFull.svelte` is a single
`<p>` of `{analysis?.recommendation}`. The shared markdown→DOMPurify helper lives at
`web/src/lib/assistant/markdown.ts` and is only imported by the assistant chat.

## Goals / Non-Goals

**Goals:**
- Prompt and sanitizer describe the same recommendation length story (budget vs safety ceiling).
- Ordinary multi-paragraph verdicts survive end-to-end and render as separate paragraphs.
- One sanitizer module owns every untrusted-model markup sink on the SPA.

**Non-Goals:**
- Reordering Stage-2 sanitize relative to Stage 3 (load-bearing for the interim stream event and
  the audit merge seed — see proposal).
- Invalidating or migrating cached `user_job_analysis` rows.
- Changing the wire type of `recommendation` or regenerating contracts.
- Feeding `recommendation` into the agent's tailoring context (already excluded).

## Decisions

### 1. Prompt budget: two or three short prose paragraphs; no headings/lists

Stated in both `stage2SystemPrompt` and `stage3SystemPrompt`, plus an explicit "do not recap
per-requirement statuses / evidence strengths — those are on the page." Stage 3 must repeat the
budget because it rewrites `recommendation`.

**Alternative considered:** one-sentence punchline only — rejected; users want the judgement and
enough context to act, and the UI already has gaps/requirements for detail lists.

### 2. Safety ceiling: `maxRecommendRunes = 1600`

~2–3 short paragraphs is roughly 1000–1300 runes; 1600 leaves headroom so the ceiling almost never
fires on in-budget answers while still bounding runaway output. Kept as a named const next to the
other matchanalysis output bounds.

**Alternative considered:** 800 — too close to a three-paragraph answer; would keep shearing.
**Alternative considered:** unbounded — rejected; the field is untrusted model text over hostile
job/company input.

### 3. Reuse `renderMarkdown` / DOMPurify; promote the module to `$lib/markdown`

Hand-splitting on `\n\n` would break on emphasis/lists the moment the model drifts. The assistant
helper already has the allowlist and tests. Promote `web/src/lib/assistant/markdown.ts` (+ its
test) to `web/src/lib/markdown.ts` and update imports — second consumer is the right moment.

Verdict card: `{@html renderMarkdown(...)}` with the existing left-rule styling on a wrapper
(`div`/`prose`-like classes), not a single `<p>`. In `stacked` mode use `text-base` instead of
`text-lg`.

### 4. No cache stamp / invalidation

Accepted: existing rows keep 400-rune stumps until CV or job content changes. Documented in
proposal; no deploy step.

## Risks / Trade-offs

- [Cached stumps linger] → Accepted; users recompute when stale for other reasons, or can force
  recompute on the page.
- [Model still overflows 1600] → Truncation remains silent (no ellipsis); rare if the prompt budget
  holds. Prompt tests assert both stages name the budget.
- [Model emits headings/lists despite the contract] → Renderer still sanitizes; visual hierarchy of
  the card may look odd. Prompt forbids them; no extra stripper.
- [Promoting markdown.ts breaks imports] → Mechanical path update; run existing markdown unit tests.

## Migration Plan

Deploy is code-only (no migration, no contract regen). Rollback: revert the commit; cached longer
verdicts remain valid JSON strings under the old UI (single `<p>` collapses whitespace but does
not break).
