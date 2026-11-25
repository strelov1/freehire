## Context

`cv-tailoring`'s evidence-citation requirement is structural: `cvedit`'s `bankGate` refuses any
claim-shaped edit whose `evidence_id` does not resolve to a candidate-asserted atom. That gate
answers "is this cited?", never "does the wording stay inside what's cited?" — `tailorPrompt`
already states the second rule in prose ("stay inside what the evidence says... never invent,
inflate or imply"), but nothing checks it.

A throwaway spike (`/tmp/crewai-spike/SPIKE_RESULTS.md`, 2026-08-09) built a two-role
writer+critic loop outside this codebase to test whether a second look catches this class of
problem. It reproduced claim inflation across three CV/JD pairs with a plain single-pass tailor
and confirmed a second look catches it — while also concluding a separate agent framework
(CrewAI, a new Python service) is not needed to capture that value; a bounded extra step in the
existing chain would likely do it. That conclusion transfers here directly: the tailoring agent
is already a single agentic loop (`internal/assistant`, `tailorPrompt`), and the codebase already
has a working precedent for a bounded, agent-driven self-check — the `job_match` tool, called
after a batch of edits, soft-capped, no server-side gate.

## Goals / Non-Goals

**Goals:**
- Give the tailoring agent a forced checkpoint to re-compare its own just-written wording against
  the atom it cited, and revise via `cv_edit` if the wording claims more than the atom supports.
- Reuse the existing evidence-read path (`bank.GetAtom`, the same call `bankGate.Publishable`
  already makes) — no new data layer, no new persistence.
- Keep the check advisory and agent-directed, the same shape as the existing `job_match`
  self-check: soft-capped rounds, the agent's own judgment to stop, no forced convergence.

**Non-Goals:**
- No second LLM pass, no separate critic process, no CrewAI or new service.
- No change to the citation-enforcement gate itself — it stays exactly as strict as it is today.
- No new server-side refusal path in `cvedit`: fidelity is a semantic judgment a deterministic
  check cannot safely arbitrate, and a false-positive hard block would refuse honest, well-worded
  bullets.
- Not retroactive: only what the agent writes going forward, in-turn — no sweep of CVs already on
  file.

## Decisions

1. **Mechanism: a new tool on the same agent, not a second LLM pass.**
   The codebase already made this call for the `job_match` self-check — methodology lives in
   `tailorPrompt`, not a second Go loop, so rules aren't split across two places and work stays
   inside the transcript. The spike's own recommendation for `matchanalysis` was the same shape:
   extend the existing chain with one bounded stage rather than stand up a new service.
   *Alternative considered:* a dedicated LLM critic call, server-side, mirroring the spike
   literally. Rejected — it would duplicate "don't fabricate/inflate" across two prompts and add
   a second model call this repo's own precedent argues against.

2. **Tool shape: read-only, re-surfaces the cited atom's own text by id.**
   Returns `{claim, context, metrics}` straight from `experience.Atom`, via the same `bank.GetAtom`
   read `bankGate.Publishable` already performs elsewhere. No new information: the agent already
   saw `claim` once (from `experience_search` or `cv_context`) before writing. The value is the
   forced re-look, not new data.
   *Alternative considered:* pass the just-written bullet text into the tool and have the tool
   itself call an LLM to judge fidelity, returning a verdict. Rejected for this change — it
   reintroduces a second model call per checkpoint for a judgment the agent, in the same turn,
   with the same context, is already positioned to make; the spike's own critic needed no
   information the tailor didn't have, only a deliberate second look at it.

3. **Bounding: mirrors the `job_match` self-check's soft cap.**
   2-3 rounds, the agent's own judgment to stop, no numeric gate — matches Scenario B of the
   spike, where an unbridgeable gap between a CV and a JD's core ask is a legitimate stopping
   point (an honest "cannot close this" note), not a bug to loop against.

4. **Scope: `cv-tailoring` (the shared prompt), not `tailor-autopilot`-only.**
   Both the conversational and autopilot rhythms run the same `tailorPrompt` — the `tailor-autopilot`
   spec's own Purpose states "the same method the conversational tailoring uses — the rhythm
   differs, not the rules." The requirement is added to `cv-tailoring`, which already owns the
   evidence-citation rule, not duplicated into `tailor-autopilot`.

## Risks / Trade-offs

- **The agent may not reliably call the new tool** — nothing forces it, by Decision 3; unlike
  citation, there is no server-side backstop for fidelity. → Mitigation: place the instruction
  directly beside the existing evidence-citation instruction in `tailorPrompt`; accept weaker
  compliance than citation gets (citation is near-100% because it is ALSO server-enforced) as the
  intended trade-off of keeping this advisory.
- **Self-review in the same context/turn may be less rigorous than a fresh critic instance** —
  the spike's critic was a separate agent with its own system prompt and no memory of drafting
  the text, not a self-review. → Mitigation: none structural; this is the open question the spike
  did not test. Accepted for v1 given the low blast radius — a missed inflation is the worst case,
  not a fabricated claim, since the evidence-id gate remains the hard floor, unchanged.
- **Extra tool round-trips cost turn budget** — `MaxSteps` ceilings exist because rounds are
  expensive, especially on a 30-step autopilot run. → Mitigation: the tool itself makes no model
  call (same cost profile as `job_match`), and the cap is 2-3 checks per run, not per bullet; only
  the agent's own reasoning about the result costs a round, same as any other tool call.

## Migration Plan

Additive only: a new tool registered alongside the existing tailoring tools, and one new
paragraph in `tailorPrompt`. No schema change, no data migration, no feature flag — this changes
agent behavior, not an API contract. Rollback is removing the tool registration and the prompt
paragraph; there is no data to unwind since the tool is read-only.

## Open Questions

- Does self-review in the same context catch as much as the spike's separate critic instance did?
  Not tested here. The closest available proxy without a live-model spike is a scripted-model
  integration test asserting the tool gets called and a deliberately-inflated draft gets revised.
- Exact tool name, and whether it should also echo back the requirement text it was meant to
  close (for parity with how `job_match` frames its result) — left to `tasks.md`.
