## 1. Extract the controlled section form (CvEditor is tailor-only now)

- [x] 1.1 Create `web/src/lib/components/cv/CvSectionForm.svelte`: move the section markup + row add/remove helpers out of `CvEditor.svelte` as a controlled component — props `bind:doc`, `bind:title`; no fetch, no autosave. Match the current keyed-each (`(entry)`).
- [x] 1.2 Fold `CvEditor`'s load + debounced autosave + save-state into the tailoring page (task 4); once the page owns `doc` and renders `CvSectionForm`, **delete `CvEditor.svelte`** (only consumer is the Edit tab this change removes). Confirm no remaining references.
- [x] 1.3 `svelte-check` clean after the extraction.

## 2. Live HTML CV preview

- [x] 2.1 Create `web/src/lib/tailor/CvHtmlPreview.svelte`: pure `{ doc, zoom? }` → resume HTML (header, summary, experience+bullets+stack, education, skills, projects, languages, certifications); CSS `transform: scale` zoom; no network. (Template does not affect the preview layout in v1 — a documented seam.)
- [x] 2.2 Preview projections (`dateRange`, `experienceHeader`, `educationLine`, `languageLabel`, `certificationLine`) live in `web/src/lib/cv.ts` as pure helpers + vitest (11 cases).
- [x] 2.3 `svelte-check` clean; a populated and an empty document both render (screenshot-verified via a throwaway route).

## 3. Right panel: rework ArtifactPanel

- [x] 3.1 Rework `web/src/lib/tailor/ArtifactPanel.svelte`: drop the `cv` (PDF iframe) and `edit` tabs and the PDF refresh/open chrome; tabs become `Templates` · `Job description` · `Verdict`.
- [x] 3.2 Templates tab: keep reusing the existing `<TemplateGallery {cvId} onSelected={…} />`; `onSelected(id)` now returns the picked id so the page keeps its own `templateId` in step (autosave writes it too) and cache-busts the PDF.
- [x] 3.3 `svelte-check` clean.

## 4. Three-column workspace page

- [x] 4.1 In `web/src/routes/tailor/[slug]/+page.svelte`, lift `doc`/`title`/`templateId` to page-owned `$state`; hydrate via `getCv(cvId)` after bootstrap/resume (reusing the resume-path record); own the debounced autosave (`updateCv`).
- [x] 4.2 On `AssistantChat.onTurnComplete`: flush any pending autosave, then refetch and replace `doc`/`title`/`templateId`; bump `pdfVersion` to cache-bust the PDF. (`refreshKey` removed — the live preview + refetch replace it.)
- [x] 4.3 Compose the three columns: left panel tabbed `Editor` (`<CvSectionForm bind:doc bind:title />`) / `Chat` (`<AssistantChat …/>`, kept mounted across tab switches); centre `<CvHtmlPreview {doc} {zoom} />` + zoom control + Download PDF; right `<ArtifactPanel …/>`. Left panel is full-width below `lg`; centre + right are `lg`-only so the chat stays reachable on mobile.
- [x] 4.4 Two resizable side panels using `clampWidth` (left width in `$state` sized off the panel's own left edge; right panel keeps its own splitter), centre `flex-1`.

## 5. Verify

- [x] 5.1 `npm run check` (svelte-check) 0 errors + `vitest` (20 passed) green; `npm run build` succeeds.
- [~] 5.2 Presentational + layout verified via headless-Chrome screenshots of a throwaway route: the centre `CvHtmlPreview` renders a classic-ats-style resume from a populated document, `CvSectionForm` renders the editor, and the responsive shell is full-width-left on mobile / three columns at `lg`. **Not driven end-to-end through the live agent/autosave path** — `/tailor/[slug]` is beta-gated and needs a running backend + authed beta user + a vacancy with a cached analysis + a stored résumé, which isn't available in this environment. Reactivity (shared `doc` → live preview) is structural (Svelte `$state` bound to the form, read by the preview).
- [x] 5.3 `/my/cvs` regression: no longer applicable — `/my/cvs/[id]` is already a redirect into the workspace and `/my/cvs` (list) uses `CvList`, untouched. `CvEditor` (the deleted component) had no other consumer.
