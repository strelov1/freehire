## Why

We generate a tailored CV and never score the artifact we produced. `atscheck.Score` runs in
exactly one place — the uploaded résumé's ATS report — so tailoring can lower a CV's
machine-readability (synonym stuffing that thins the summary, an overflowed page, contacts pushed
out of reading order by a template change) and the candidate never learns. They apply with a
document we made worse and we told them nothing.

The fix is cheap because every part already exists: `atscheck.Score` is pure, `cv.TypstRenderer`
renders the document, and `resume.ExtractPDFText` reads a PDF's text layer. What is missing is the
loop that closes them — compile the CV, read it back the way a parser reads it, and say what
changed.

## What Changes

- **Score a tailored CV from its rendered PDF's text layer**, not from its stored JSON. The JSON is
  what we meant; the text layer is what an ATS sees. This is the only way the score can catch a
  layout regression, a non-ATS-safe template, or a contact block that stopped being first.
- **Compare against the base CV the copy was made from**, rendered with the same template and
  scored against the same vacancy, so the delta isolates the tailoring's contribution rather than
  mixing in a template switch or a different keyword baseline.
- **Serve an overall delta and a per-category delta** over the five existing `atscheck` categories.
  A negative overall delta is reported as a warning that names the category that fell.
- **Surface it in the tailoring workspace.** Informational only: nothing is blocked, no export is
  gated, no confirmation is demanded. The candidate already has undo; this gives them the fact.
- **Recompute on read.** Nothing new is stored, so the delta always reflects the current document,
  the current template and the current scoring rules.

Not in this change, and deliberately: the tailoring agent does not receive the score. Handing a
metric to the thing being measured invites it to optimise the metric instead of the CV, and that
tradeoff deserves its own change with its own evidence.

## Capabilities

### New Capabilities
- `tailor-ats-delta`: scoring a tailored CV from its rendered artifact's text layer, comparing it
  against the base CV under identical rendering and keyword conditions, and reporting the overall
  and per-category change (including the warning when readability fell).

### Modified Capabilities
None. `cv-ats-score`'s requirements are about the profile's uploaded CV and stay true as written;
this change adds a second, separately-specified subject rather than altering the first.
`tailor-workspace`'s existing requirements (the three-column surface, the preview, the template
picker) are unchanged — the delta's presentation contract is specified by the new capability that
owns it, so there is one home for the feature instead of a requirement split across two specs.

## Impact

- **New**: an owner-scoped read endpoint under `/me/cvs/:id/` serving the delta; a small domain unit
  that turns two `atscheck.Report`s into a comparison.
- **Reused unchanged**: `internal/atscheck` (`Score`, the five categories), `internal/resume`
  (`ExtractPDFText`), `internal/cv` (`TypstRenderer.Render`, `ResolveTemplate`).
- **Web**: the tailoring workspace gains a delta indicator; `cmd/gen-contracts` picks up the new
  wire shape.
- **Cost**: a request scores two documents, so it pays two Typst compiles and two `pdftotext` runs.
  Whether that is served synchronously per request, and what bounds it, is a design question — the
  numbers, not the shape, are what design.md has to settle.
- **Dependency**: scoring now needs poppler in the API image on a path that previously only needed
  it for résumé upload. The production image already installs `poppler-utils`; a deployment without
  it must degrade to "no delta available", never to a 500.
