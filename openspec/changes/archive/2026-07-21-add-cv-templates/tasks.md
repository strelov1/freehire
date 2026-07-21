## 1. Template registry with metadata

- [x] 1.1 Introduce a `TemplateInfo{ID, Label, Style string; ATSSafe bool}` value in `internal/cv/template.go`; back the registry with a `[]TemplateInfo` slice (initially the single `classic-ats` entry, ATSSafe true). Keep `TemplateIDs()` and `ResolveTemplate` behavior identical (derive ids from the slice). Add `Templates()` returning the metadata. Unit test: `TemplateIDs()` still contains `classic-ats`, `ResolveTemplate("")`/`ResolveTemplate("classic-ats")` succeed, unknown id yields `ErrUnknownTemplate`, and `Templates()` reports `classic-ats` ATSSafe.

## 2. New Typst templates

- [x] 2.1 Add `internal/cv/templates/centered.typ` (single-column serif, centered name/contacts/headings, no color) and register it in the metadata slice (ATSSafe true). Unit test: `ResolveTemplate("centered")` returns non-empty source.
- [x] 2.2 Bundle Liberation Sans: commit `internal/cv/fonts/LiberationSans-Regular.ttf`, `LiberationSans-Bold.ttf`, and the SIL OFL license; `//go:embed` them in the `cv` package. Teach `TypstRenderer.Render` to materialize the embedded fonts into the render sandbox and pass `--font-path`. Render test (skip without Typst): compiling a minimal template that sets `font: "Liberation Sans"` under `--ignore-system-fonts` succeeds and yields extractable text (proving the font wiring); existing serif render test still passes.
- [x] 2.3 Add `internal/cv/templates/modern-sans.typ` (single-column, `Liberation Sans`, uppercase left-aligned name, no color) and register it (ATSSafe true). Unit test: `ResolveTemplate("modern-sans")` returns non-empty source.
- [x] 2.4 Add `internal/cv/templates/sidebar.typ` (two-column serif via Typst `grid`: left sidebar = contact/links/skills/languages, right = summary/experience/education/projects, no color) and register it with ATSSafe **false**. Unit test: `ResolveTemplate("sidebar")` returns non-empty source and `Templates()` reports it not ATSSafe.
- [x] 2.5 Add a render smoke test (guarded to skip when no Typst binary is available) that compiles every registered template against a representative `Document` and extracts the text layer, asserting the candidate name and a skill are present — including for `sidebar`.

## 3. Templates list endpoint

- [x] 3.1 Add `ListCVTemplates` handler in `internal/handler/cv.go` returning `{"data": [{id,label,style,ats_safe}, ...]}` from `cv.Templates()`. Register `GET /api/v1/cv-templates` in `handler.go` behind the same `saved` + `cvGate` middleware as the other CV routes. Handler/integration test: authorized beta user gets all registered templates with the four fields (classic-ats ats_safe true, sidebar false); non-beta user gets 403.

## 4. Set-template endpoint

- [x] 4.1 Add a `SetCVTemplate` sqlc query + `cvStore.SetTemplate(ctx, id, userID, templateID)` store method that updates only `template_id` for an owned CV (returns not-found for a foreign/missing id), mirroring the existing `SetSession`. Store/integration test: setting a template leaves title + document unchanged; a foreign id returns not-found.
- [x] 4.2 Add a `SetCVTemplate` handler in `internal/handler/cv.go` for `PUT /api/v1/me/cvs/:id/template` (body `{template_id}`), validating via `validCVTemplate` and routing through `cvStore.SetTemplate`; register it in `handler.go` under `saved` + `cvGate`. Handler test: valid id updates and returns success; unknown id → 400; foreign id → 404.

## 5. Static preview thumbnails

- [x] 5.1 Add `cmd/cv-previews`: a dev tool that renders a fixed sample `Document` through each template in `cv.Templates()` to SVG via the Typst CLI (`compile --format svg`), writing `web/static/cv-previews/<id>.svg`. Factor the sample document + per-template SVG generation so it is unit-testable. Unit test (temp output dir, guarded on Typst availability): the generator emits exactly one SVG per registered template id and none extra.
- [x] 5.2 Run the generator and commit `web/static/cv-previews/{classic-ats,centered,modern-sans,sidebar}.svg`. Add a `make cv-previews` target and a note (in the relevant AGENTS.md) that template edits require regenerating previews.

## 6. Frontend template discovery

- [x] 6.1 Add a `CvTemplate` type (`id`, `label`, `style`, `ats_safe`) in `web/src/lib/cv.ts`; add `listCvTemplates()` and `setCvTemplate(id, templateId)` methods in `web/src/lib/api.ts` (GET `/api/v1/cv-templates`, PUT `/api/v1/me/cvs/:id/template`). Verify with `svelte-check`/build.

## 7. Templates gallery tab

- [x] 7.1 Add a "Templates" tab to `web/src/lib/tailor/ArtifactPanel.svelte` rendering a thumbnail gallery (one `/cv-previews/<id>.svg` per template from `listCvTemplates()`), highlighting the CV's current `template_id`, with a "may not parse cleanly in some ATS" hint on non-ATS-safe entries. Clicking a thumbnail calls `setCvTemplate`, updates the highlight, and bumps `cvVersion` so the CV iframe re-renders. Degrade gracefully if the list fetch fails. Verify via build + a visual check that selecting a template switches the PDF preview.
