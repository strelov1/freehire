## Context

`add-cv-pii-masking` masked the CV on the extraction and fit-analysis paths. Two surfaces still
send the raw CV / user PII to an external model: the ATS qualitative review (`atscheck.Analyze`
posts the raw CV) and the CV-tailoring **agent** (it reads the full CV — contacts included —
through its scoped key and runs its own model). The fit chain also re-masks the raw CV on every
run even though the structured résumé already carries the fit signal PII-free.

The CV builder's `cv.Document` already separates a `Header` (contact block) from the body, and
the tailoring agent authenticates with a short-lived owner-scoped API key — both are levers.

## Goals / Non-Goals

**Goals:**
- The raw CV reaches an external model in exactly one place: résumé extraction on upload (masked
  by the local detector). Every other surface uses de-identified derived data.
- Keep the published guarantee ("PII does not leave the service") true across all CV surfaces.

**Non-Goals:**
- Changing résumé extraction (`resume-structured-profile`) — it stays the single masked point.
- Masking the tailoring agent's prompts token-by-token — the structured `Header`/body split makes
  that unnecessary.
- A perfect scrub of a name that a user hand-wrote into an experience bullet (best-effort; rare).

## Decisions

**D1 — Fit analysis reads the structured résumé, not the raw CV.** The structured résumé (title,
years, experience highlights, skills, education) carries the fit signal; the raw CV adds PII and
formatting noise. Feed the structured résumé with contact fields excluded; drop `writeCV` and the
whole per-analysis redactor/detector/restore. *Alternative:* keep masking the raw CV — rejected:
more code, more model load, and it re-sends data we don't need. *Trade-off:* fit quality now rides
on extraction quality (it already fed the chain as primary-grade context; extraction is required
anyway for the contact split).

**D2 — ATS review reads the structured résumé.** `atscheck`'s LLM review judges writing quality
(verbs, quantification), which lives in the faithfully-copied `highlights`. Feed structured, not
raw. *Trade-off:* it loses the raw-layout/garbled-text judgement; the deterministic ATS layer
(near-empty text ⇒ scanned/garbled) keeps the extreme case. *Alternative:* mask the raw CV like
fit did — rejected for the same reason as D1.

**D3 — The tailoring agent gets a contact-stripped Document.** On the tailoring-key read path,
omit `Header.{full_name,email,phone}`; on the patch path, reject those fields. Distinguish the
agent (API key) from the owner (cookie). The real contacts live only in our DB and the rendered
PDF. *Alternative:* mask/restore the whole CV text for the agent — rejected: the agent is
autonomous/multi-turn and the structured `Header` split makes stripping trivial and exact.

**D4 — The detector's footprint shrinks to extraction only.** After D1–D3 nothing downstream calls
the detector; `matchanalysis` and `atscheck` no longer take a `pii.Detector`.

## Risks / Trade-offs

- [Fit/ATS quality depends on extraction] → extraction is already required (contacts); a missing
  structured résumé degrades to no analysis / deterministic score, not an error.
- [ATS loses layout/garbling judgement] → the deterministic ATS-readiness layer already flags the
  scanned/empty extreme; the LLM review keeps its writing-quality value from `highlights`.
- [Name hand-written into a body bullet leaks to the tailoring agent] → rare; accepted best-effort,
  noted. The `Header` split covers the overwhelming case.
- [Owner vs agent auth must be distinguished correctly] → gate on the auth mechanism (cookie =
  owner, tailoring key = agent); a mistake would either over-expose or break the owner's own view —
  covered by tests on both paths.

## Migration Plan

1. Ship D1–D4 behind the existing behaviour: fit/ATS switch to structured input; tailoring-key
   reads strip contacts. No schema change, no new env.
2. The detector service stays as-is (still used by extraction). No redeploy of the sidecar needed.
3. Rollback: revert the code; extraction masking is untouched throughout.
