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
