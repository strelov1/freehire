## Why

The CV PII masking (change `add-cv-pii-masking`) de-identifies the CV on the two paths that
send it to the LLM — résumé extraction and fit analysis. But it left the raw CV / user PII
reaching an external model on other surfaces: the ATS qualitative review (`atscheck`) still
sends the raw CV, and the CV-tailoring **agent** reads the full CV (contacts included) through
its scoped key. And the fit analysis re-masks the raw CV on every run when it doesn't need the
raw CV at all — the structured résumé already carries the signal, PII-free.

The clean invariant: **the raw CV reaches an external model in exactly one place — résumé
extraction on upload, where our local detector masks it. Everything downstream works on the
de-identified structured résumé (or, for tailoring, the CV body without the contact block).**

## What Changes

- `job-fit-analysis` (`matchanalysis`): score the fit from the **de-identified structured
  résumé** (its contact fields excluded), not the raw CV. Remove the per-analysis masking —
  the redactor, the detector dependency, and the restore path — since no raw CV is sent.
- `cv-ats-score` (`atscheck`): the optional LLM review reads the **structured résumé** (the
  faithfully-copied experience highlights carry the writing to judge), not the raw CV text.
- `cv-tailoring`: the tailoring **agent** receives the CV **without its contact block**
  (`full_name`/`email`/`phone`); contact fields are neither readable nor patchable via the
  short-lived tailoring key. The real contacts stay in our DB and appear only in the rendered
  output — the agent's model never sees them.
- `cv-pii-masking`: state the single-point-of-use — the detector runs only at extraction;
  downstream surfaces consume de-identified derived data, not the raw CV.
- **Unchanged:** `resume-structured-profile` (extraction keeps the local-model masking — the
  one raw-CV→LLM point).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `job-fit-analysis`: fit is scored from the de-identified structured résumé, not the raw CV; the raw CV is no longer sent and no longer masked per-analysis.
- `cv-ats-score`: the LLM qualitative review reads the structured résumé, not the raw CV.
- `cv-tailoring`: the tailoring agent's credential cannot read or write the CV contact block.
- `cv-pii-masking`: the detector is a single-point-of-use at extraction; downstream is de-identified by construction.

## Impact

- Code: `internal/matchanalysis` (drop CVText from the chain + remove redactor/detector/restore); `internal/atscheck` (take structured input); `internal/handler/cv.go` + `cv_tailor.go` (contact-strip on the tailoring-key read/patch path); `internal/handler` wiring.
- Behavior: fit analysis and the ATS review now depend on the structured résumé (a missing extraction degrades them, as it already may); the ATS review loses raw-layout/garbling judgement (kept by the deterministic ATS layer). The tailoring agent sees a contact-less CV.
- No new infra; the detector footprint shrinks to the extraction path only.
