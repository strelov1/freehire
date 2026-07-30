## Context

`atscheck.Score` is pure and I/O-free and runs in exactly one place: `internal/handler/ats_report.go`
scores the **uploaded** résumé's extracted text against the top-20 skills of a role facet. The CV
builder is a different world — a structured `cv.Document` rendered to PDF by `cv.TypstRenderer` —
and nothing there is ever scored. Tailoring therefore has no feedback loop: a run can thin the
summary, overflow a page, or move the contact block behind a sidebar, and the candidate applies with
a worse artifact than they started with.

Everything needed already exists and is already shaped for this:

- `atscheck.Score(cvText string, cvSkills, roleTopSkills []string) Report` — five categories summing
  to 100, plus `Potential`.
- `cv.Renderer` is an **interface** on `cvHandlers.cvRenderer`, left nil when no typst binary was
  resolved (the existing 501 gate for `RenderCVPDF`). Fakeable in tests.
- `resume.ExtractPDFText([]byte) (string, error)` is exported (`internal/resume/resume.go:271`) over
  poppler's `pdftotext`.
- `jobReader.GetJob(ctx, id) (db.Job, error)` is already a field on `cvHandlers`, and `db.Job` carries
  canonical `Skills`.
- `skilltag.Parse(text, skilltag.WithResumeAcronyms())` is how the existing report derives a CV's own
  skills from its text.

Measured before designing: a Typst compile of a representative CV takes **30–90 ms**
(`go test ./internal/cv/ -run Render|Typst` — four templates compile in 0.15 s total), and
`TestTypstRendererProducesExtractableATSText` already asserts a rendered CV's text layer carries the
name and skills. So the honest path is also a cheap one.

## Goals / Non-Goals

**Goals:**

- Tell the candidate what tailoring did to their CV's machine-readability, measured on the artifact
  an ATS will actually parse.
- Isolate the tailoring's contribution: hold template, margins and keyword baseline constant across
  both sides of the comparison.
- Add no storage, no migration, and no new external dependency.

**Non-Goals:**

- The tailoring agent does not receive the score. Handing a metric to the thing being measured
  invites keyword stuffing against the metric; that trade-off needs its own change and its own
  evidence.
- No LLM. The delta is deterministic; the existing optional LLM review stays where it is, on the
  uploaded-résumé report.
- No gating. Nothing about export, download or saving changes.
- No new ATS category and no re-weighting. The five categories are the contract.

## Decisions

### Score the rendered PDF's text layer, not the stored document

**Chosen:** render → `resume.ExtractPDFText` → `skilltag.Parse` → `atscheck.Score`.

**Alternative rejected:** add a plain-text projection of `cv.Document` and score that. It is cheaper
(no compile, no poppler) and it is exactly wrong: it would score what we intended to render. A
sidebar template that buries contacts, a page that overflowed, a heading the active template drops —
all invisible. The whole value of this change is reading the artifact back the way a parser reads it,
which is the practice the upstream `ai-job-search` template converged on independently
(`tools/verify_pdf.py` plus an ATS text-layer check after compiling).

### Compute synchronously, cache nothing

Two documents per request means two compiles and two `pdftotext` runs — under 400 ms with the
measured numbers, on a read the client makes when a workspace opens and when a run ends.

**Alternative rejected:** memoize the base CV's report by `(cv id, updated_at, template, margins,
vacancy)`. It halves the work on repeat reads and buys a second source of truth for a score, plus a
cache key with five components that must all be right. Not for savings we have not measured as a
problem. If it ever becomes one, this is the escape hatch — and the key is written down here.

### The comparison is a pure function in `internal/atscheck`

`atscheck` owns `Report` and the five categories, so it owns what a difference between two reports
means: a new `delta.go` with the wire type and a pure `Compare(base, tailored Report)`. Table-testable
with no renderer, no HTTP, no DB.

Orchestration — resolve both CVs, render both, extract, score, compare — lives in a new
`internal/handler/cv_ats_delta.go` over the collaborators `cvHandlers` already holds.

**Alternative rejected:** a new `internal/atsdelta` package. It would own one struct and one function
and would have to import `atscheck` for both; the seam adds a name, not a boundary.

### The keyword baseline is the bound vacancy's canonical skills

Both sides are scored against `db.Job.Skills` for the vacancy the tailored CV is bound to, read
through the existing `jobReader`.

**Alternatives rejected:** the role's top-20 facet skills (what the uploaded-résumé report uses) —
needs a search round-trip and is strictly less precise than the vacancy the CV is *for*; and
`matchanalysis.RequirementMatch` — LLM-derived, so the baseline could shift between two reads and
move the delta while the CV stood still. A deterministic delta needs a deterministic baseline.

### The route is cookie-only, and that is the enforcement

`GET /me/cvs/:id/ats-delta` registers with `mw.cookie`. The tailoring agent authenticates with a
short-lived CLI credential, so a cookie-only route is closed to it by transport — the "the agent does
not see the score" non-goal is enforced by the router, not asked for in a system prompt. This follows
the project's existing line: rules that are ours to enforce live in the service path, not in prompt
text.

### Unavailability is a 200, not a 501

The response carries `available: false` plus a reason when the renderer is absent or a render fails —
the same shape as the existing report's `has_cv: false`. `RenderCVPDF` answers 501 when
`cvRenderer` is nil because rendering *is* what the caller asked for; the delta is an accessory read
on a page that must keep working, so it degrades instead.

### The wire type must be added to the contracts generator

`cmd/gen-contracts` lists `IncludeFiles: []string{"atscheck.go"}` for the atscheck package. A new
`delta.go` is invisible to codegen unless it is added there, and the failure is silent — a TS
contract that simply lacks the type.

## Risks / Trade-offs

- **The baseline is the base CV as it stands now, not as it stood when the copy was made** → If the
  candidate edits their base CV after tailoring, the delta compares against something that was never
  the copy's origin. Snapshotting the base at `Tailor` time would fix it and would mean a stored
  document per tailored CV, contradicting recompute-on-read for a case (editing the base mid-tailor)
  we have no evidence of. Accepted and documented; the response names which base CV it compared
  against so the number is never anonymous.
- **A pruned vacancy can promote an orphan to "the base CV"** → Pre-existing, found while reviewing
  this change, and NOT fixed here. `cvs_job_id_fkey` is `ON DELETE SET NULL`, so when `cmd/prune`
  hard-deletes a job, its tailored copy keeps existing with `job_id` NULL — and
  `GetBaseCVByUser` defines the base as the newest `job_id IS NULL` row. A recently-tailored
  orphan therefore outranks the real base and becomes the baseline for every other tailored CV's
  delta: wrong numbers, silently, with no crash. The orphan itself cannot be compared (its NULL
  `job_id` is the 409 path), and it can never be its own baseline, so the damage is bounded to a
  misleading comparison. The root cause is shared with `cv.Store.Tailor`, which seeds new copies
  from that same query, so this predates the delta and fixing it properly means schema work (an
  explicit `is_base` flag, or an FK that does not orphan) — its own change, not a rider on this one.
- **A template switch is attributed to tailoring** → Prevented by design: the base is rendered with
  the tailored copy's template and margins, so a switch moves both sides together.
- **Two renders on every workspace open** → Measured cheap (30–90 ms per compile). Watch the tailor
  workspace's latency after ship; the memoization key above is the fix if it bites.
- **Goodhart, if the score ever reaches the agent** → Blocked twice over: the non-goal, and a
  cookie-only route the agent cannot call.
- **A future scoring-rule change silently moves everyone's delta** → Acceptable, and preferable to
  stored scores that would silently disagree with the current rules. Recompute-on-read means one
  truth at a time.
- **`pdftotext` missing on some deployment path** → Degrades to `available: false`, never a 500. The
  production image already installs `poppler-utils` for résumé upload.

## Migration Plan

No schema change, no migration, nothing persisted — the change is code-only and additively
deployable. Rollback is a revert: with no stored delta there is nothing to clean up and no way for
old data to become inconsistent. The web surface reads a new endpoint, so an SPA deployed ahead of
the API sees a 404 and must render the delta as absent, which is the same path as
`available: false`.

## Open Questions

None blocking. Deferred deliberately: whether the tailoring agent should ever see the score. Revisit
only with evidence that candidates ignore a displayed regression — and then as a change that
confronts the keyword-stuffing incentive head-on, not as a quiet route change.
