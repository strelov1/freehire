## Context

Builds on the merged verdict redesign (#433): `internal/verdict` computes a top-20
role-skill breakdown with `strong`/`hidden`/`missing` status + must-have/stack/
coherence stats; `internal/atscheck` scores five weighted categories. Both are pure
and dictionary-first. `internal/cvsection` already yields the CV's `declared`/`body`/
`all` skill sets. These four additions are all deterministic, mirroring
`internal/skilltag`/`location`/`classify`.

## Goals / Non-Goals

**Goals:** market-combination signal (bundles); soften binary gaps with an `adjacent`
band; typed actionable advice; a summary keyword-density ATS check. All deterministic.

**Non-Goals:** archetype detection, ghost-job/legitimacy scoring, conversion
pre-filtering, interview story-bank (career-ops features unrelated to CV-vs-market
scoring); LLM-derived adjacency; auto-generating a reworded CV.

## Decisions

**1. Curated bundle dictionary (`internal/skillbundle`), not derived co-occurrence.**
A handful of market-recognised bundles → member skill slugs. Coverage of a bundle by
a CV = |members ∩ cvAll| / |members|; a bundle is "covered" at ≥ `BundleCoveredPct`
(start 50%). *Why:* matches the repo's dict-first convention (precision over a noisy
derived graph); a fixed, legible set beats live co-occurrence mining for a first cut.
Seed bundles: `genai-core`, `cloud-ops`, `web-stack`, `data`, `ml`.

**2. `adjacent` as a fourth skill status, from a curated adjacency dictionary.**
`adjacentTo[roleSkill] = {close candidate skills}`. Classification precedence:
`strong` (declared) → `hidden` (body) → `adjacent` (an adjacent skill is in the CV) →
`missing`. `adjacent` counts toward neither must-have coverage nor stack-match (the
candidate lacks the exact skill) but is surfaced as "close, reframe-able" rather than
a hard miss. *Why:* a curated, symmetric-ish adjacency map is deterministic and
auditable; a 4th enum value is additive on the wire (status is a free string).

**3. Advice is status-keyed and names the concrete lever.** `adjacent` → "you have
<close>, reframe the overlap or ramp up"; `hidden` → surface in Skills; `missing` →
learn + evidence. Pure templates.

**4. Summary keyword-density line item in `atscheck`.** Reuse `cvsection`-style
heading detection to isolate the summary section; award points when it carries ≥ N of
the CV's skill tags. Slots into the existing Content or Section category (keep the
100-point total by re-balancing item weights within that category, not adding a 6th
category).

## Risks / Trade-offs

- **Curated dicts need seeding + upkeep** → keep them small and AI/backend-focused
  first; note the seam for expansion (mirrors skilltag). A dict change needs no
  migration (verdict/atscheck recompute live).
- **`adjacent` could over-trigger** if the adjacency map is too loose → seed
  conservatively (only genuinely substitutable pairs) and treat `adjacent` as
  not-covered so it never inflates must-have/stack numbers.
- **Bundle overlap with stack-match** (both read the CV's skills) → bundles group by
  market combination (a different lens: "you have the ops bundle"), stack-match is
  flat top-20 breadth; label them distinctly.
- **Summary-density re-balancing** must keep category maxima summing to 100 →
  covered by an `atscheck` test asserting Σmax = 100.

## Open Questions

- Final `BundleCoveredPct` and the exact bundle/adjacency seed contents (calibrate
  against real CVs, e.g. the one used to validate must-have=50%).
