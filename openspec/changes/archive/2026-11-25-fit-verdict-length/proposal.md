## Why

The fit analysis' `recommendation` — the text the analysis page leads with under the heading
"The verdict" — is amputated mid-word on every substantial analysis. `maxRecommendRunes` bounds it
at 400 runes and `llm.TrimTruncateRunes` slices there with no ellipsis, so a reader cannot tell the
server cut the sentence from the model stopping. The prompt is the other half of the fault: Stage 2
asks for `a single "recommendation" string` and states no length at all, so the model has no budget
to stay inside and reliably sails past a bound it was never told about.

The damage is not only cosmetic. Stage 2's verdict is sanitized *before* it is marshalled into the
Stage-3 audit prompt, so the adversarial pass is handed a recommendation that dies mid-word and
asked to refine it — the skeptic stage reasons over broken input on every long verdict.

## What Changes

- **State a length budget in the prompt.** Stage 2 and Stage 3 both name how long a verdict should
  be — two or three short prose paragraphs, no headings and no lists — so the model's target and
  the server's bound finally describe the same thing. Both stages, because the audit rewrites the
  field and a budget stated in only one would drift.
- **Turn `maxRecommendRunes` back into a safety guard.** Raise it well clear of the stated budget so
  it bounds a hostile or runaway model instead of shearing every ordinary answer. The two numbers
  keep distinct jobs: the prompt governs style, the cap governs safety.
- **Render the verdict as multi-paragraph prose.** The verdict card is a single `<p>`, so paragraph
  breaks collapse today. It renders through the existing sanitize-then-`{@html}` path already used
  for the assistant's answers, rather than a second hand-rolled renderer. The card also sizes down
  in the narrow `stacked` panel, where a three-paragraph verdict at `text-lg` is a wall.
- **The verdict stops recapping the requirement table.** The budget is spent on the hiring judgement,
  not on re-listing per-requirement statuses the page already renders with evidence-strength badges.

Deliberately excluded: **the Stage-2/Stage-3 double truncation is not reordered.** The sanitized
Stage-2 verdict is load-bearing twice — it is the `dimensions` interim event streamed to the browser,
where the bound *is* the injection guard, and it is the seed the audit's JSON is unmarshalled over, so
a field the audit omits keeps a sanitized value. Sanitizing later would either stream unbounded model
text or restore unsanitized text through the merge. A cap generous enough never to fire is the fix.

Also excluded: **cached analyses are not invalidated.** The cache is stamped on CV upload time, job
`content_hash`, and `LLM_MODEL`; none change here, so existing rows keep serving their 400-rune stump
and are not reported stale. They heal as CVs and postings change. Accepted knowingly.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-fit-analysis`: the `recommendation` field gains a stated length contract — a prompt-side
  budget distinct from the sanitizer's safety ceiling — and the served verdict is multi-paragraph
  prose rather than one sentence.
- `assistant-output-rendering`: the sanitizer policy is no longer the assistant's alone. The fit
  verdict becomes a second consumer, so the requirement covers every surface that renders untrusted
  model prose as markup rather than naming the chat.

## Impact

- `internal/matchanalysis/analyzer.go` — `stage2SystemPrompt`, `stage3SystemPrompt`.
- `internal/matchanalysis/matchanalysis.go` — `maxRecommendRunes`.
- `web/src/lib/components/MatchAnalysisFull.svelte` — the verdict card's rendering and stacked sizing.
- `web/src/lib/assistant/markdown.ts` — promoted out of `$lib/assistant/` now that it has a consumer
  outside the assistant; nothing about the policy is assistant-specific.

No migration: `user_job_analysis.analysis` is `jsonb`. No `cmd/gen-contracts` run: `recommendation`
stays a `string`. No effect on the agent's tailoring context, which already excludes the field with a
test naming the reason — so a longer verdict costs no agent tokens.
