# Tailor cold-start: assemble from the bank, don't gate on analysis — design

Sub-project 3 of the match/tailor merge thread. Builds on, and partially supersedes,
`2026-08-08-match-tailor-merge-design.md` / PR #1658 — see "Relationship to PR #1658"
below. Discovered live-testing that PR: the base/tailored CV seed dumps the *entire*
experience bank unfiltered, and the existing JD-aware curation ("Tailor it for me" /
autopilot) is a manual, opt-in action rather than the cold-start default.

## Problem

Two issues found together during live QA of PR #1658:

1. **Seeding is unbounded.** `experienceFromBank()` (`internal/experience/professional.go`)
   takes every publishable atom under every employment with no cap. As a candidate
   accumulates more banked achievements over time (chat confirmations, repeated
   tailoring sessions), every subsequently-seeded CV gets longer and less curated —
   the opposite of what a résumé needs.
2. **The tool that fixes this already exists but isn't the default.** "Tailor it for
   me" (`web/src/lib/tailor/autopilot.ts`'s `openingActions()`) already runs an agent
   turn that goes through the vacancy's requirements against the bank and picks
   relevant evidence — exactly the JD-aware curation a seed lacks. It's a manual
   button inside the chat, reachable only after the cold-start bootstrap (currently
   gated on a cached fit analysis — PR #1658's "analyzing" flow) already completed.

## Decision: replace the cold-start gate, not add to it

**The AI fit analysis no longer blocks starting the workspace.** Opening
`/tailor/[slug]` for the first time on a vacancy immediately starts an autopilot run —
the JD-aware bank curation — building the tailored CV live. The fit analysis (verdict,
score, gaps) runs **in the background, in parallel**, and populates the Job Match tab
whenever it lands — exactly the existing "surfaces the delta without being asked"
pattern already used for the ATS delta, just applied to the fit analysis too.

This is not additive to PR #1658's "analyzing" screen — it replaces it. Once the
bootstrap no longer requires a cached analysis to exist, the `409 "run the fit
analysis first"` path that screen exists to handle can no longer occur, and the
screen becomes dead code.

## Scope

**In scope:**
- `internal/cv`: the `Tailor()`/`Store` bootstrap no longer requires a cached fit
  analysis. It seeds/creates the base CV as today, then — instead of stopping there —
  kicks off the autopilot curation run as part of the SAME cold start, before handing
  the workspace to the candidate.
- The fit analysis (`internal/matchanalysis`) is triggered in the background at the
  same moment, independently, not blocking the response. Reuses the existing
  streamed-compute path; only the trigger site and blocking-ness change.
- Frontend: the CV preview shows the autopilot run happening live — bullets and
  sections filling in as the agent works — not a single before/after swap.
- Frontend: PR #1658's `status: 'analyzing'` branch, `retryBootstrapAfterAnalysis`,
  and the inline `MatchAnalysisFull` render in `+page.svelte` are removed (unreachable
  once the backend never 409s for "no analysis").

**Out of scope, deferred:**
- Capping bullet count in `experienceFromBank()` for the *base* (vacancy-less) CV —
  a base CV still has no JD to curate against, so it keeps the current unfiltered
  seed (a real remaining gap, not solved by this change — noted for a future pass).
- The colored underline overlay (sub-project 2, already speced separately).

## Open questions for the implementation plan

These need answers before task-writing, not before this design is approved — they're
implementation-shaped, not architecture-shaped:

1. **Does the autopilot run consume anything metered today**, and should making it
   the automatic cold-start default (rather than an explicit click) count against
   some quota, the way the fit analysis has its 10/30-day cap? Needs checking
   `internal/llmkey`/wherever autopilot's cost is currently attributed.
2. **Live-streaming the build into the preview is new work, not a rewire.** Today an
   agent turn refetches and replaces the whole `Document` once, at turn end
   (`web/src/routes/tailor/[slug]/+page.svelte`'s agent-turn handling). Showing the
   candidate their CV assembling section-by-section needs the document (or enough of
   it) to update incrementally as the agent's tool calls land, not just at the end.
   Scope how incremental that needs to be (per-section vs. per-bullet) against how the
   assistant's turn/tool-call events are already surfaced in chat.
3. **Failure/degradation:** what does cold start show if autopilot fails outright (no
   LLM configured, bank empty, agent error)? Likely: fall back to the current
   unfiltered `Seed()` bootstrap rather than leaving the candidate on a dead screen —
   this is the same "best-effort degrade, never block" instinct the rest of this
   codebase already follows everywhere else.
4. **Résumé-less users**: `cv-tailoring`'s *other* 409 ("add a résumé first," no base
   CV to seed from) is untouched by this change — still a hard stop, still unrelated
   to the fit-analysis gate being removed.

## Relationship to PR #1658

#1658 is not wasted work — it fixed a real thing (no `/match` page hop, redirects,
the retry race-condition bug) and should merge on its own merits. This design's
frontend piece (dropping the `'analyzing'` branch) is a small, mechanical follow-up
*on top of* #1658, once it's landed — not a reason to hold #1658 back or unwind it.
