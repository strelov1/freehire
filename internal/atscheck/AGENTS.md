# ATS-readiness scoring conventions

## Scope
A deterministic 0–100 ATS-readiness score for a CV's plain text: five weighted categories
with per-item point attribution (atscheck.go). An optional LLM layer refines the Content
Quality category (analyzer.go), and `Compare` builds the Delta between a base and a
tailored CV's reports (delta.go). Consumers: `GET`/`POST /me/profile/ats-report`
(internal/handler/ats_report.go, routes at resume.go:87-88) and
`GET /me/cvs/:id/ats-delta` (cv_ats_delta.go, route at cv.go:205).

## Always true
- **Text-only input.** The CV text comes from a plain-text PDF extractor, so layout facts
  (multi-column, tables, images) are NOT detectable — only what text allows. The strongest
  readable signal is emptiness: a scanned/image CV yields almost no text and fails
  `machine_readable`, the single biggest ATS killer (atscheck.go:9-13;
  `minReadableWords` = 30, atscheck.go:91).
- **Pure and I/O-free** — `Score(cvText, cvSkills, roleTopSkills)` (atscheck.go:136)
  mirrors internal/verdict. The handler supplies the text, the parsed skills
  (`skilltag.Parse`), and the role's top in-demand skills.
- **Five categories, server-owned weights, maxima summing to 100:** Keyword Strength 40,
  Format Compliance 20, Section Completeness 15, Content Quality 15, Length & Density 10
  (atscheck.go:44-50 plus the per-item weights). `Overall` is the sum of category scores;
  `Potential` adds back every recoverable point, capped 100 (`recompute`,
  atscheck.go:158-176).
- **The analyzer NEVER sends the raw CV.** De-identification is enforced by the argument's
  type — a `resumeextract.Professional` names the fields a model may see — not by
  sanitizing: a field added to the structured résumé is withheld until somebody adds it
  there too (analyzer.go:53-60). A nil client makes `Analyze` a no-op returning
  `(nil, nil)`, so callers degrade to the deterministic score (analyzer.go:61-64).
- **Model output is untrusted:** the score is clamped and suggestions are trimmed,
  length-bounded and capped (analyzer.go:82-95), and `Review.UnmarshalJSON` flex-decodes a
  score returned as `"85"` or `"85/100"` instead of aborting the whole review over one such
  slip (flexdecode.go:13-24). The CV projection itself is capped at 24k runes
  (analyzer.go:14-15).
- **`ApplyReview` replaces the Content Quality category** with the LLM's score, attaches
  the suggestions and re-sums; it also flips `Reviewed`, which the SPA reads to switch
  "Run" to "Re-run" (atscheck.go:391-413).
- **A Delta reports only categories present in BOTH reports** — a category on one side
  alone is not a difference, and reporting it against an implied zero would invent a change
  nobody made (delta.go:13-16). `WorstCategory` is set only when the overall fell, and an
  equal drop resolves to the earlier category in the tailored report's own order, never map
  iteration order (delta.go:44-79). Only the TAILORED side's line items travel — a
  before/after checklist is a diff nobody asked for (delta.go:33-37).

## How it works
`POST /me/profile/ats-report` runs the LLM review under the caller's own gateway
credential — `h.atsAnalyzer.As(h.llm.bind(...))` (ats_report.go:89) — so the spend is
attributed per-user per internal/llmkey; `cfg.LLM` nil means deterministic only
(handler.go:362). The ats-delta endpoint renders base and tailored CVs and scores both
through one helper (`scoreRenderedCV`, cv_ats_delta.go:117-122) so the two sides can never
disagree about what the text says. `LineItem`/`Status` are the shared wire shape that
`cmd/gen-contracts` excludes cvmatch's `lineitem.go` for — both scorers' rows render
through one frontend component; if the shapes ever diverge, that exclusion has to go.
