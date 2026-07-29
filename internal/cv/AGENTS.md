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
