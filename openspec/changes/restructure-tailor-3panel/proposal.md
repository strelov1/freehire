## Why

The tailoring workspace today is two columns — chat on the left, a tabbed artifact panel
(CV-as-PDF · Job description · Verdict) on the right — with the structured editor buried as one
of those right-hand tabs and the CV shown only as a slow PDF iframe. A candidate tailoring a CV
wants to see the document update *as they type* and to work the deterministic fields and the
agent side by side. This restructures the surface into the familiar three-column resume-builder
shape: edit on the left, live preview in the centre, context (templates, JD, verdict) on the
right.

## What Changes

- Restructure `/tailor/[slug]`'s ready state from two columns into **three**:
  - **Left — tabbed:** `Editor` (the structured CV section form) · `Chat` (the tailoring agent).
  - **Centre — live CV preview:** a new **HTML render** of the CV `Document` that updates
    instantly as the left-hand form is edited, with a zoom control and a **Download PDF** button
    (the existing Typst endpoint). There is **no in-page PDF preview** — the centre is HTML only.
  - **Right — tabbed:** `Templates` (the template picker) · `Job description` · `Verdict`.
- **Lift the CV `Document` to page-owned state** so the editor and the centre preview share one
  in-memory object: typing re-renders the preview with no server round-trip; autosave persists in
  the background; an agent turn refetches and replaces the shared document.
- **Extract the structured section form** out of `CvEditor.svelte` into a controlled
  `CvSectionForm.svelte` (`bind:doc`). `CvEditor` stays a thin load-and-autosave container for
  `/my/cvs`; the tailoring page owns load/save itself and reuses `CvSectionForm`.
- Keep the existing **`Templates` gallery** (`TemplateGallery` over `listCvTemplates` /
  `setCvTemplate`, four registered templates) in the right panel; choosing one sets `template_id`,
  which the PDF download honours. The centre HTML preview does not visually swap per template in
  this iteration (a documented seam for later).
- **Beta-gated. No backend / API / DB changes and no migration** — every endpoint already exists
  (bootstrap, `updateCv`, `getCv`, `cvPdfUrl`, `GET /jobs/:slug`, the cached analysis).

Explicitly **out of scope** (deferred): AI Review, Suggested Edits, a Resume Score meter,
per-section AI buttons, section reorder/rename, a multi-template gallery, and per-template HTML
preview parity.

## Capabilities

### New Capabilities
<!-- None. This restructures existing workspace behaviour; no new capability is introduced. -->

### Modified Capabilities
- `tailor-workspace`: the workspace becomes a three-column surface (left-tabbed
  Editor/Chat, centre live HTML CV preview + Download PDF, right-tabbed
  Templates/Job description/Verdict); the structured editor moves into the left tab group and its
  edits reflect live in the centre preview; a template picker is added.

## Impact

- **Frontend (web/) only:**
  - New `web/src/lib/tailor/CvHtmlPreview.svelte` (Document → resume HTML + zoom).
  - New `web/src/lib/components/cv/CvSectionForm.svelte` (controlled section form, `bind:doc`),
    extracted from `CvEditor.svelte`; `CvEditor.svelte` is then **removed** (its only consumer,
    the panel's Edit tab, goes away; its load/autosave folds into the page).
  - `web/src/lib/tailor/ArtifactPanel.svelte` reworked into the right-hand panel: drop the `cv`
    and `edit` tabs, keep the existing `Templates` gallery + Job description + Verdict.
  - `web/src/routes/tailor/[slug]/+page.svelte` owns the shared `doc` state, three-column layout,
    two resizable splitters (reusing the tested `clampWidth`), and refetch-on-agent-turn.
- **No Go / API / DB changes. No migration.**
- **Tests:** pure logic (any new preview projections, existing `clampWidth` / `splitRequirements`)
  → vitest; components → `svelte-check` + visual verify.
- **Risk:** folding `CvEditor`'s load/autosave into the page — mitigated by carrying the exact same
  debounce, snapshot-diff, and best-effort flush, and visual-verifying edit → autosave → the
  preview and PDF reflect it.
