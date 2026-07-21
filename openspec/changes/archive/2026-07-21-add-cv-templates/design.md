## Context

CV rendering is server-side: `internal/cv/renderer.go` (`TypstRenderer`) shells
out to the Typst CLI, writing the CV `Document` as `data.json` and the resolved
template as `template.typ` into a temp `--root` dir, then compiling to PDF. The
template registry lives in `internal/cv/template.go` as `var templateIDs
[]string` (one entry, `classic-ats`) with `ResolveTemplate(id)` reading
`templates/<id>.typ` from an embedded FS. `template_id` already flows end-to-end:
DB column → `cv.Meta` → handler request/response → `CvEditor.svelte` (bound to a
hidden `templateId`, no UI). The frontend shows the rendered PDF in an `<iframe>`
(`ArtifactPanel.svelte`), so there is no Svelte markup to change per template —
each template is one `.typ` file.

The registry comment explicitly names the extension seam: "extending this list
and dropping a matching templates/<id>.typ file." This change walks that seam.

## Goals / Non-Goals

**Goals:**
- Ship 3 new no-color, simple templates (`centered`, `modern-sans`, `sidebar`)
  next to `classic-ats`.
- Let the UI discover templates from the server (id, label, style, `ats_safe`)
  instead of hard-coding.
- Present templates as a visual thumbnail gallery (the reference tool's layout)
  where the user picks one and the PDF preview updates.
- Keep `classic-ats` the default; existing CVs render unchanged.

**Non-Goals:**
- No per-template configurable options (accent color, font size). Templates are
  fixed, no-color.
- No per-user live thumbnails (rendering the user's own CV in every template).
  Previews are static images of a fixed sample résumé, like the reference tool.
- No DB schema changes and no new runtime dependencies (only a new sqlc query).

## Decisions

**1. Registry carries metadata, not just ids.** Replace `var templateIDs
[]string` with a slice of a small `TemplateInfo{ID, Label, Style string;
ATSSafe bool}` value. `ResolveTemplate` and `TemplateIDs()` keep working (the
latter derived from the slice) so existing callers and validation are untouched;
a new `Templates()` returns the metadata for the endpoint. Rationale: the label
and `ats_safe` flag are display concerns the server already owns (it owns the
templates), so listing them server-side keeps the UI dumb and avoids drift.
Alternative considered — a parallel map keyed by id — rejected as two sources of
truth for the same set.

**2. New list endpoint at `GET /api/v1/cv-templates`.** Registered with the same
auth + beta gate (`saved`, `cvGate`) as the other CV routes. A top-level path
(not `/me/cvs/templates`) avoids colliding with the `/me/cvs/:id` param route.
Returns `{"data": [{id,label,style,ats_safe}, ...]}` following the list
response-shape convention. It is static registry data, so no per-user work.

**3. Templates are self-contained `.typ` files.** Because the renderer only
copies `template.typ` + `data.json` into the sandboxed temp `--root`, a Typst
`#import` of a shared helper module would not resolve. Each template therefore
duplicates the ~8-line data-reading preamble (`json("data.json")`, `s`, `arr`,
`daterange`). This is deliberate: templates stay independently readable and the
renderer stays a two-file sandbox. Extracting a shared preamble is not worth
teaching the renderer to stage extra files.

**4. Set-template endpoint, separate from document save.** Add `PUT
/api/v1/me/cvs/:id/template` (body `{template_id}`) mirroring the existing
`SetCVSession` pattern: a `cvStore.SetTemplate` method + a sqlc query updating
only the `template_id` column, owner-scoped, validating the id. Rationale: the
gallery flips a template without holding or re-sending the full document, so it
can't clobber concurrent edits, and it stays a cheap one-column write. The
existing full `PUT /me/cvs/:id` keeps carrying `template_id` for the editor's
document autosave — no conflict because `ArtifactPanel` tabs mount via `{#if}`,
so `CvEditor` re-reads the current `template_id` on every mount.

**5a. Gallery lives in a new "Templates" tab of `ArtifactPanel.svelte`.** This
matches the reference layout and reuses the panel's existing `cvVersion`
cache-bust: selecting a template calls `api.setCvTemplate(cvId, id)`, then bumps
`cvVersion` (via an `onTemplateChange`-style local update) so the CV iframe
re-renders. The tab fetches the templates list and the CV's current
`template_id` (via `getCv`) to highlight the active one. Non-ATS-safe templates
show an inline hint. Because the standalone `/my/cvs/:id` route only redirects
into `/tailor/<slug>?cv=<id>`, the panel is the single home for the gallery — no
second surface to build.

**5b. Static SVG previews from a fixed sample résumé.** A dev tool
`cmd/cv-previews` loads one hard-coded representative `Document`, and for each
template in `cv.Templates()` renders it to SVG via the Typst CLI (`compile
--format svg`), writing `web/static/cv-previews/<id>.svg`. The SVGs are committed;
the frontend maps `id → /cv-previews/<id>.svg`. Rationale: previews are identical
for every user and change only when a template changes, so static committed
assets beat rendering at request time; SVG (not PNG) is diffable, crisp, and
small. The generator iterates the registry, so the set can't drift; a generator
unit test (temp output dir) asserts it emits exactly one SVG per registered
template. Regenerating after a template edit is a documented `make cv-previews`
step. Alternative — extend the `Renderer` interface with an SVG format — rejected
to keep the production render interface PDF-only; the generator is dev-only and
tolerates a little duplication of the compile shell-out.

**6. Template layouts (all no-color):**
- `centered` — single-column serif (Libertinus); name centered, contacts
  centered inline under it; section headings centered with a rule. ATS-safe.
- `modern-sans` — single-column sans-serif using the bundled Liberation Sans
  (decision 7); uppercase name left-aligned; left-aligned section labels.
  ATS-safe.
- `sidebar` — two-column serif via a Typst `grid`: left sidebar (contact, links,
  skills, languages), right main column (summary, experience, education,
  projects). NOT marked ATS-safe.

**7. Bundle Liberation Sans for the sans template.** The Typst binary embeds only
`Libertinus Serif`, `New Computer Modern`, and `DejaVu Sans Mono` — no
proportional sans — and the renderer runs `--ignore-system-fonts`, so a genuine
sans template needs a bundled face. Embed Liberation Sans Regular + Bold (SIL
OFL, Helvetica-metric-compatible, so it reads as the classic professional resume
sans) into the `cv` package via `//go:embed`, plus its license. At render time
the renderer materializes the font files into the sandbox and adds `--font-path
<dir>` so `#set text(font: "Liberation Sans")` resolves. The same extraction is
reused by the preview generator. Rationale: font variety (serif vs sans) is the
core visual contrast between templates; one committed libre font is the minimal
cost. Alternatives — DejaVu Sans Mono (monospace, unusual for a CV) or a
serif-only "modern" variant (weak contrast) — rejected as not delivering the
visual distinction that justifies multiple templates.

## Risks / Trade-offs

- **Sidebar text-extraction order** → Typst emits grid cells in layout order, so
  a two-column PDF may interleave sidebar/main text when linearized. Mitigation:
  the template is explicitly flagged not-ATS-safe in the UI; the spec only
  requires that name + skills remain *extractable* (present), not correctly
  ordered — a render+extract test asserts presence.
- **Bundled-font resolution** → the sans face resolves only if the renderer
  actually stages the embedded fonts and points `--font-path` at them. Mitigation:
  a render test compiles a template requesting `Liberation Sans` under
  `--ignore-system-fonts` and asserts a successful compile with extractable text,
  proving the font wiring works locally and in the prod image.
- **Registry shape change** → touching `templateIDs` could break `ResolveTemplate`
  callers. Mitigation: keep `TemplateIDs()` and `ResolveTemplate` signatures
  identical; only their backing data changes, covered by existing + new unit
  tests.

## Migration Plan

Additive only. No data migration: `template_id` column and defaults already
exist. Deploy is a normal build (new `.typ` files are embedded at compile time).
Rollback = revert the commit; existing CVs keep their `template_id`, and any CV
that had been switched to a new template would 400 on render after rollback
(acceptable: pre-release beta feature, and the value can be reset to
`classic-ats`).

## Open Questions

- None blocking. Font choice for `modern-sans` is resolved during implementation
  by the render test (decision 6 / risk 2).
