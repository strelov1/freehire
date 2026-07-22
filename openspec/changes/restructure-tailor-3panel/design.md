## Context

`/tailor/[slug]` is a beta, full-width surface. Its ready state is currently two columns:
`<AssistantChat>` on the left and `<ArtifactPanel>` (tabs: CV-as-PDF · Job description · Verdict)
on the right. The structured editor exists as `CvEditor.svelte` (a self-contained load +
debounced-autosave component, also used standalone on `/my/cvs/[id]`) and is surfaced as a
right-panel tab. The CV `Document` is the single wire shape rendered to PDF server-side by Typst
(`internal/cv/renderer.go`, `templates/<id>.typ`; only `classic-ats` registered). There is no
HTML render of a CV anywhere.

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

### 2. Extract `CvSectionForm` from `CvEditor`

Split `CvEditor.svelte` into:
- `CvSectionForm.svelte` — controlled, presentational: `bind:doc`, `bind:title`,
  `bind:templateId`, plus optional `embedded`. No data fetching, no autosave. This is the section
  markup lifted verbatim.
- `CvEditor.svelte` — thin container for `/my/cvs`: keeps the current load + debounced-autosave +
  save-state chrome, and renders `<CvSectionForm bind:doc bind:title bind:templateId />`.

The tailoring page reuses the same autosave logic (extracted to a tiny helper or duplicated
minimally) since it now owns `doc`.

- **Why:** two consumers need the same fields bound to a document they don't own; a controlled
  component is the clean seam. `/my/cvs` stays behaviourally identical (same props flow through
  the container).
- **Alternative — leave `CvEditor` intact, mount it in the left tab and mirror its `doc` out via
  `onSaved`:** couples the preview to save timing (defeats decision 1); rejected.

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

Drop the CV/PDF tab (moved to centre), add a `Templates` tab. Keep Job description and Verdict
(reuse `splitRequirements`). The Templates tab lists `TemplateIDs()` (exposed to the client via
the existing CV record's `template_id` plus a small static list, or a tiny read — no new
write endpoint; selection just sets `templateId` in the shared state, which autosaves).

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
