## Context

`/tailor/[slug]` is a beta, full-width surface. Its ready state is currently two columns:
`<AssistantChat>` on the left and `<ArtifactPanel>` (tabs: CV-as-PDF · Job description · Verdict)
on the right. The structured editor exists as `CvEditor.svelte` (a self-contained load + debounced-autosave
component) surfaced as the panel's `Edit` tab; `/my/cvs/[id]` is now only a redirect into the
workspace, so `CvEditor` is used *nowhere else*. The panel also already has a working `Templates`
tab (`TemplateGallery.svelte` over `listCvTemplates` / `setCvTemplate`, with SVG thumbnails) and
four registered templates (`classic-ats`, `centered`, `modern-sans`, `sidebar`). The CV
`Document` is the single wire shape rendered to PDF server-side by Typst
(`internal/cv/renderer.go`, `templates/<id>.typ`). There is no HTML render of a CV anywhere.

This change reshapes the ready state into three columns and adds the one genuinely new piece: an
HTML render of the CV `Document`. Everything else (chat, section form, JD, verdict, template
list, PDF endpoint) already exists and is recomposed. No backend work.

## Goals / Non-Goals

**Goals:**
- Three-column workspace: left-tabbed Editor/Chat, centre live HTML CV preview + Download PDF +
  zoom, right-tabbed Templates/Job description/Verdict.
- The editor and the centre preview share one in-memory `Document`, so typing re-renders the
  preview with zero server round-trip.
- Reuse the existing section form on both the workspace and `/my/cvs` without duplicating it, and
  without changing `/my/cvs` behaviour.
- No Go/API/DB changes; no migration.

**Non-Goals:**
- No in-page PDF preview (centre is HTML only; PDF is a download).
- No per-template HTML preview parity (the HTML preview is one clean layout; template choice only
  affects the downloaded PDF).
- No AI Review / Suggested Edits / Resume Score meter / per-section AI buttons / section
  reorder-rename / multi-template gallery — all deferred.

## Decisions

### 1. Page-owned shared `Document` state (instant preview)

The `/tailor/[slug]/+page.svelte` becomes the owner of the client-side CV state: `doc`, `title`,
`templateId` as `$state`. It passes `bind:doc` to the section form and `{doc}` to the preview, so
both read one object. The page owns load (`getCv`) and the debounced autosave (`updateCv`).

- **Why over refetch-per-change:** typing must feel instant like the reference; a shared in-memory
  object gives that for free, where a server refetch would lag ~1s and lose scroll on re-render.
- **Two-writer coordination:** the human writes via the form (autosave, 800ms debounce); the agent
  writes server-side, only in response to a user turn. On `AssistantChat.onTurnComplete` the page
  refetches `getCv(id)` and replaces `doc`. The user is not typing during an agent turn, so the
  overwrite is safe; a pending autosave is flushed before/independently of the refetch.
- **Alternative considered — keep `CvEditor` self-contained, preview refetches on `onSaved`:**
  simpler and zero-refactor, but laggy and flickery; rejected for the premium feel the reference
  implies (chosen explicitly with the user).

### 2. Extract `CvSectionForm`, fold `CvEditor` into the page

`CvEditor.svelte` is used only by the panel's `Edit` tab, which this change removes (the editor
moves into the left panel against the shared `doc`). So:
- Lift the section markup + row helpers into a controlled `CvSectionForm.svelte` — `bind:doc`,
  `bind:title`, no fetch, no autosave.
- Fold `CvEditor`'s load + debounced-autosave + save-state into the tailoring page (which now owns
  `doc`), then **delete `CvEditor.svelte`** as orphaned by this change.

- **Why:** the editor and the centre preview must share one in-memory `doc` (decision 1), so the
  page — not a self-contained component — has to own load/save; `CvSectionForm` is the presentational
  seam both the left tab and (transitively) the preview read.
- **Why delete rather than keep as a container:** with `/my/cvs/[id]` a redirect, no standalone
  consumer remains; a kept-but-unused container is dead code this change would create.
- **Alternative — leave `CvEditor` intact in the left tab and mirror its `doc` out via `onSaved`:**
  couples the preview to save timing (defeats decision 1); rejected.

### 3. `CvHtmlPreview.svelte` — a pure `Document → HTML` render

A Svelte component that takes `doc` (and `templateId`, currently unused for layout) and renders a
clean, ATS-style resume: header (name, contacts, links), summary, experience (role/company/dates
+ bullets + stack), education, skills groups, projects, languages, certifications — the same
section set the form and Typst cover. Zoom via CSS `transform: scale`. No network, no PDF.

- **Why client-side Svelte over a server HTML endpoint:** the user asked for HTML-only preview and
  the `Document` is already on the client; a server render would add an endpoint for no benefit.
- **Fidelity:** models the `classic-ats` spirit, not pixel-parity with Typst. The download PDF is
  the source of truth for the printed artifact.

### 4. Right panel = reworked `ArtifactPanel`

Drop the `cv` (PDF iframe) and `edit` tabs — the CV now renders in the centre and edits in the
left panel. Keep the three context tabs: `Templates` (the existing `TemplateGallery`, unchanged),
`Job description`, and `Verdict` (reuse the same components). On a template switch,
`TemplateGallery` already persists via `setCvTemplate`; its `onSelected` tells the page to refetch
so the shared `templateId` (and the Download-PDF output) reflect the new choice.

### 5. Layout & splitters

Three columns via fl: left panel fixed-by-splitter width, right panel fixed-by-splitter width,
centre `flex-1`. Reuse the tested `clampWidth(px, min, max)` for both splitters (pointer-capture,
as in the current single splitter). Two independent widths in `$state`.

## Risks / Trade-offs

- **[Extracting the section form from a shared component]** → keep `CvEditor`'s container
  behaviour identical (same autosave debounce, same save-state UI, same props) and visual-verify
  `/my/cvs` create/edit/save as a regression gate.
- **[Agent turn overwrites an in-flight human edit]** → autosave debounce is 800ms and the user is
  not typing while awaiting an agent turn; flush any pending save, then refetch. Acceptable for a
  beta single-user surface; revisit only if it bites.
- **[HTML preview drifts from the Typst PDF]** → expected and accepted; the PDF download is the
  printed source of truth and the preview is explicitly "close, not identical". Documented seam to
  add per-template HTML later.
- **[Templates tab implies a gallery]** → only `classic-ats` is registered; the tab shows the
  registered set honestly and is the seam for more, not a fake gallery.

## Migration Plan

Pure frontend, behind the existing beta gate. No migration, no rollback concerns beyond reverting
the branch. `/my/cvs` regression is the one thing to verify because of the `CvEditor` extraction.

## Open Questions

- None blocking. The exact resume HTML layout in `CvHtmlPreview` (section order, date/location
  placement, typography) is a deliberate implementation-time design choice, refined during build.
