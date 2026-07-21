## Why

The tailor CV feature ships exactly one visual template (`classic-ats`). Users
who want a résumé that doesn't look identical to everyone else's have no choice,
even though the rendering pipeline was explicitly built with a template registry
seam for this. Offering a few distinct, simple, no-color layouts is a high-ROI
win: the plumbing (per-CV `template_id`, `ResolveTemplate`, Typst renderer)
already exists — only new template files, a list endpoint, and a selector UI are
missing.

## What Changes

- Add three new Typst templates alongside `classic-ats`, all no-color and
  deliberately simple:
  - `centered` — single-column serif, name and contacts centered.
  - `modern-sans` — single-column sans-serif, uppercase name, left-aligned.
  - `sidebar` — two-column serif with a left sidebar (contact, skills, links)
    and a right main column (experience, education, projects). Flagged as a
    layout that **may not parse cleanly in some ATS**.
- Extend the `templateIDs` registry so all four resolve and validate.
- Expose the registry over a read endpoint (`GET /api/v1/cv-templates`) so the
  UI can list available templates instead of hard-coding them, including a
  human-facing label, a short style descriptor, and an `ats_safe` flag.
- Add a dedicated endpoint to set only a CV's `template_id`
  (`PUT /api/v1/me/cvs/:id/template`) so the gallery can switch template without
  re-sending the whole document.
- Generate a static preview thumbnail (SVG) per template from a fixed sample
  résumé, committed as a frontend asset, so the UI shows a visual gallery.
- Add a "Templates" tab to the tailoring artifact panel: a thumbnail gallery
  where the user picks a template; selecting one persists it and re-renders the
  PDF preview. This replaces the current hidden hard-coded `template_id`
  pass-through as the way users choose a template.
- Relax the PDF ATS-contract requirement so "single column" describes the
  ATS-safe templates specifically, while all templates keep an extractable text
  layer.

No breaking changes: `classic-ats` stays the default and existing CVs render
unchanged.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `cv-builder`: the template registry grows from one to four templates; a new
  read endpoint lists templates with display metadata; a new endpoint sets a
  CV's template alone; templates are discoverable in the UI as a thumbnail
  gallery; the ATS PDF contract is reworded so single-column applies to ATS-safe
  templates rather than all templates.

## Impact

- Backend: `internal/cv/template.go` (registry + template metadata),
  `internal/cv/templates/*.typ` (three new files),
  `internal/handler/cv.go` + route wiring (list endpoint + set-template
  endpoint), a `SetTemplate` store method + sqlc query.
- Preview assets: a dev tool (`cmd/cv-previews`) renders each template from a
  fixed sample document to SVG; committed under `web/static/cv-previews/`.
- Frontend: a new "Templates" gallery tab in
  `web/src/lib/tailor/ArtifactPanel.svelte`, `web/src/lib/api.ts` (list + set
  template), `web/src/lib/cv.ts` (template type).
- No DB schema change: `template_id` column and end-to-end flow already exist;
  only a new read/update query is added.
- No new dependencies: reuses the existing Typst binary (SVG output for previews,
  PDF for live render).
