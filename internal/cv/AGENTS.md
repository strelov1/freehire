# internal/cv

Per-user structured CVs (CRUD + seed + tailoring) and on-demand PDF rendering. The HTTP
surface lives in `internal/handler/cv.go`; this package owns the domain, storage, and
rendering.

## Templates

Templates are Typst source files under `templates/<id>.typ`, embedded via `//go:embed`. The
registry is `templates []TemplateInfo` in `template.go` (id, label, style, `ats_safe`).
`ResolveTemplate(id)` defaults an empty id to `classic-ats` and rejects unknown ids with
`ErrUnknownTemplate`; `Templates()` exposes the metadata for the UI gallery and preview
generation.

**Adding a template:**
1. Add `templates/<id>.typ` — a self-contained file reading the CV from `json("data.json")`
   (helpers `s`/`arr`/`daterange` are duplicated per file; the renderer only stages
   `template.typ` + `data.json` + fonts, so Typst `#import` of a shared module won't resolve).
2. Append a `TemplateInfo` entry to `templates`. Mark `ATSSafe: false` for anything that is
   not single-column with standard headings (e.g. `sidebar`).
3. Run `make cv-previews` to regenerate `web/static/cv-previews/<id>.svg` (the gallery
   thumbnails). A preview is committed for every registered id — the generator iterates the
   registry so the set can't drift.

## Rendering

`TypstRenderer` shells out to the Typst CLI in a sandboxed temp `--root` with
`--ignore-system-fonts`; user data goes through `data.json` (never argv). `compile` is shared
by `Render` (PDF, live) and `GeneratePreviews` (SVG, `cmd/cv-previews`).

Fonts: the Typst binary embeds no proportional sans, so Liberation Sans (SIL OFL) is bundled
under `fonts/`, staged into the sandbox, and exposed via `--font-path`. A template that wants
sans uses `#set text(font: "Liberation Sans")`.

## ATS delta (tailored vs base)

`GET /me/cvs/:id/ats-delta` reports what tailoring did to a CV's ATS readiness. The scoring
input is the **rendered PDF's text layer**, not the stored document: the orchestration lives in
`internal/handler/cv_ats_delta.go` (render → `resume.ExtractPDFText` → `skilltag.Parse` →
`atscheck.Score`), and the two-report comparison is `atscheck.Compare`. A document field the
active template never renders therefore earns nothing — `sidebar` has no certifications block,
and the handler test pins exactly that.

Four things the code decides and a reader would otherwise have to infer:

- **The comparison holds everything but content constant.** The base CV is rendered with the
  *tailored copy's* template and margins (an in-memory copy — the stored base is never touched),
  and both sides are scored against one keyword baseline: the bound vacancy's canonical
  `jobs.skills`. Not the role facet's top skills, and not the LLM's requirement match, whose
  drift would move the delta while the CV stood still.
- **The baseline is the base CV as it stands NOW.** There is one base CV per user
  (`cv.Store.BaseCV`), not a snapshot taken when the copy was made, so editing the base moves
  the delta. Deliberate — a snapshot would mean storing a second document per tailored CV —
  and pinned by `TestATSDelta_ComparesAgainstTheCurrentBase`.
- **The route is cookie-only, and that is the enforcement.** The tailoring agent authenticates
  with a CLI credential, so the gate is what keeps the score away from the thing being measured.
  Widening it to `mw.key` hands the agent a metric to optimise; `TestCVRegister_ATSDeltaIsCookieOnly`
  is the tripwire.
- **Unavailable is a 200, not a 501.** No renderer or a failed compile answers
  `available: false` with a reason. `RenderCVPDF` 501s for the same condition because rendering
  is what that caller asked for; the delta is an accessory read on a page that must keep working.

Nothing is stored: it is recomputed per request, so a scoring-rule change needs no migration.

## Autopilot runs

A tailored CV carries what the last unattended run left behind: `autopilot_report` (one entry
per requirement the run considered, with an outcome from a fixed vocabulary) and
`autopilot_undo` (the document as it stood before the run's first edit). The wire shape lives
in `autopilot.go` — it is generated to TypeScript — while `autopilot_store.go` holds what may
be persisted and the owner-scoped writes that persist it. Keeping them in separate files is
what stops the client seeing rules that are ours to enforce.

Three rules the code encodes rather than documents:

- **Nothing is coerced.** A status outside `closed_bank` / `closed_candidate` / `open` /
  `not_reached` is refused with the valid ones named, because that message is the model's only
  route to correcting itself inside the turn. Text is trimmed and truncated silently — those
  are display concerns.
- **A report is replaced whole.** There is no partial update: a requirement closed later from
  the candidate's own words arrives as the same list with one entry changed, so the stored
  value is always the current truth and there is one write path instead of two.
- **A revert clears the report with the document.** `RevertAutopilot` restores the snapshot and
  nulls both columns in one owner-scoped statement; a CV with no snapshot matches nothing and
  yields `ErrNoAutopilotRun` (the handler's 409) rather than blanking the document with NULL.
  Keeping the log would leave the workspace claiming edits that no longer exist.

The snapshot is taken fresh at the start of EVERY run, so "undo the run" always means the
document as the last run found it.

Two known edges, both deliberate:

- **Runs are not serialised per CV.** Two runs started at once (a double click, two tabs) each
  snapshot, and the second captures a half-edited document — so undoing returns to the middle of
  the first run. The workspace disables its entry points while a turn is in flight; a server-side
  lock is machinery this has not yet earned.
- **A run lays down its own plan.** The handler writes the vacancy's requirements as
  `not_reached` before the turn starts, because a run that exhausts the step cap gets its final
  model call with no tools offered and therefore cannot report. The agent's report replaces the
  plan wholesale.
