## Context

`internal/matchanalysis` runs a fixed three-stage LLM prompt-chain per (user, job): Stage 1
(Extract & Match) turns the posting into a requirement table classified against the candidate's
structured résumé; Stage 2 scores six recruiter dimensions; Stage 3 is an adversarial audit of
Stage 2. The served `Analysis` struct is cached, keyed on CV upload time + job `content_hash` +
model, and streamed to the frontend over SSE (`analyzer.go`'s `AnalyzeStream`, consumed by
`web/src/lib/matchAnalysis.ts`'s `reduceMatchEvent`).

The chain only ever runs for a candidate with banked/structured experience (`candidateContext`
returns empty and the whole chain no-ops otherwise) — this design adds no new gate, it rides the
existing one.

## Goals / Non-Goals

**Goals:**
- Surface up to 5 short, JD-quote-grounded interpretive signals about pace, ownership
  expectations, team stage, or culture, as part of the existing fit analysis.
- Zero additional LLM calls, zero new cache-invalidation logic, zero new endpoints.
- Degrade cleanly to nothing when the posting is too generic to read anything from.

**Non-Goals:**
- Not a standalone feature usable without a fit analysis (no bank/résumé, no signals) — see the
  brainstorming decision already made with the user.
- Not a fixed-vocabulary classification (no enum of signal "types"). Each signal is freeform
  quote + freeform interpretation, sanitized by bounds only, the same tier as the existing
  `Recommendation` and dimension `comment` fields — not the controlled-vocabulary tier that
  `status`/`priority`/`evidence_strength` belong to.
- Not a deterministic/dictionary-based extraction. This is interpretive text, same category as
  the rest of Stage 1/2/3's free-text output, and is expected to vary slightly across runs the
  same way `recommendation` already does.

## Decisions

**Extend Stage 1's existing call rather than add a new stage.** Stage 1 already reads the full
job description to extract requirements; hidden signals are read from the same text with no
extra context needed. Alternatives considered: a dedicated Stage 1.5/new stage (rejected — an
extra model round-trip per analysis for output that shares its entire input with Stage 1), or
a deterministic Go-side keyword dictionary (rejected in the brainstorming discussion — shallower
than an LLM read of actual phrasing, and this package's existing free-text fields are already
LLM-derived rather than dict-based).

**Wire shape: `Signal { Quote string; Insight string }`, field `HiddenSignals []Signal` on
`Analysis`.** Mirrors the existing `Requirement`/`Dimension` naming pattern in
`matchanalysis.go`. JSON keys: `hidden_signals`, each entry `{"quote": ..., "insight": ...}`.

**Bounds:** cap at 5 signals (`maxSignals = 5`, same order of magnitude as `maxStrengths`/
`maxGaps` at 6). `Quote` bounded to 200 runes (mirrors `maxReqTextRunes`, since a quote is
JD-length text like a requirement's text). `Insight` bounded to 200 runes (mirrors
`maxListItemRunes`, since it is a single-sentence interpretation like a strength/gap entry).

**Sanitize by dropping, not coercing.** A signal with a blank quote or blank insight is dropped
entirely (same rule `sanitizeRequirements` applies to a blank requirement text) rather than
substituting a placeholder — an interpretive claim with nothing to ground it is not useful
half-formed.

**No new SSE event kind.** The existing `EventRequirements` event (emitted right after Stage 1
sanitization, `analyzer.go` `AnalyzeStream`) gets one more field, `HiddenSignals []Signal`,
alongside `Requirements`. Both are Stage 1's output and become visible to the frontend at the
same moment; a new event kind would only duplicate that timing for no benefit.

**Persisted on `Analysis`, not held stage-1-only.** `HiddenSignals` threads through
`buildAnalysis` into the final `Analysis` exactly as `RequirementMatch` does today, so it is
part of the cached/served payload and needs no cache-key change (still triple-stamped on CV
upload time, job `content_hash`, model — the signals are a pure function of the same job text
and the same Stage 1 call already covered by that stamp).

**Frontend placement:** the Hidden Signals section renders directly after the Requirement Match
table and before Strengths/Gaps — it reads as "extra color on the posting itself" rather than
part of the scored verdict, so it sits with the other posting-derived content (the requirement
table) rather than the recruiter-verdict content. Omitted entirely (no heading, no empty state)
when `hidden_signals` is an empty array.

Correction found during implementation: the standalone `/match/[slug]` page named in the
proposal no longer exists — it now redirects to `/tailor/[slug]` (the fit-analysis surface
moved into the Tailor workspace, per the already-shipped "Fit analysis surfaces in the
tailoring workspace" requirement). The actual integration point is
`web/src/lib/components/MatchAnalysisFull.svelte`, the single component both the Tailor
workspace's `ArtifactPanel` and `JobDrawer` embed — placement logic is unchanged (same
section, same rule), only the file path differs from what the proposal assumed.

## Risks / Trade-offs

- **[Risk] Stage 1's completion grows slightly (one more JSON key with up to 5 entries), adding
  marginal latency to a stage already budgeted for the requirement table.** → Mitigation: same
  call, same timeout budget (`stageAttempts`/existing per-call deadline); an extra bounded field
  is a small fraction of Stage 1's existing output size (up to 30 requirements today).
- **[Risk] Freeform interpretive text carries the same prompt-injection surface as the rest of
  Stage 1's untrusted-input handling (the JD is caller-uncontrolled but adversary-reachable via
  job ingest).** → Mitigation: identical bounding/truncation discipline already applied to
  every other free-text field in this package (`llm.TruncateRunes`, count caps); no new
  injection surface, just one more bounded field.
- **[Risk] A candidate with no banked résumé never sees hidden signals, even though they are
  purely JD-derived and don't need a CV.** → Accepted per the already-approved brainstorming
  decision: this rides the existing fit-analysis gate rather than becoming a second, ungated
  code path and cost surface.

## Migration Plan

No data migration. Purely additive wire field; existing cached `Analysis` rows simply lack
`hidden_signals` until the job's analysis is next recomputed (cache miss/refresh), where the
zero-value/omitted field renders as no signals shown — never an error.

## Open Questions

None outstanding — all decisions above were confirmed with the user before this document was
written.
