## 1. Extract the controlled section form (reuse, no /my/cvs regression)

- [ ] 1.1 Create `web/src/lib/components/cv/CvSectionForm.svelte`: move the section markup out of `CvEditor.svelte` as a controlled component — props `bind:doc`, `bind:title`, `bind:templateId`, `embedded?`; no fetch, no autosave. Keep the row add/remove helpers with it.
- [ ] 1.2 Refactor `CvEditor.svelte` into a thin container: keep load (`getCv`), debounced autosave (`updateCv`), save-state chrome + back-link; render `<CvSectionForm bind:doc bind:title bind:templateId {embedded} />`.
- [ ] 1.3 `svelte-check` clean; visual-verify `/my/cvs` create → edit a field → autosave → reload shows persisted (regression gate for the extraction).

## 2. Live HTML CV preview

- [ ] 2.1 Create `web/src/lib/tailor/CvHtmlPreview.svelte`: pure `{ doc, templateId?, zoom? }` → resume HTML (header, summary, experience+bullets+stack, education, skills, projects, languages, certifications); CSS `transform: scale` zoom; no network.
- [ ] 2.2 If any non-trivial projection is needed for the preview (e.g. formatting dates/links), put it in `web/src/lib/cv.ts` as a pure helper + vitest.
- [ ] 2.3 `svelte-check` clean; render a populated document and an empty document without errors.

## 3. Right panel: rework ArtifactPanel

- [ ] 3.1 Rework `web/src/lib/tailor/ArtifactPanel.svelte`: drop the CV/PDF tab; tabs become `Templates` · `Job description` · `Verdict` (keep the JD text and the `splitRequirements` verdict).
- [ ] 3.2 Templates tab: list registered templates (default `classic-ats`), indicate the CV's current one, and emit selection so the page sets `templateId` in shared state (autosaves). No new write endpoint.
- [ ] 3.3 `svelte-check` clean; visual-verify tab switching and a template selection round-tripping to `template_id`.

## 4. Three-column workspace page

- [ ] 4.1 In `web/src/routes/tailor/[slug]/+page.svelte`, lift `doc`/`title`/`templateId` to page-owned `$state`; load via `getCv(cvId)` after bootstrap/resume; own the debounced autosave (`updateCv`).
- [ ] 4.2 On `AssistantChat.onTurnComplete`: flush any pending autosave, then refetch `getCv(cvId)` and replace `doc`/`title`/`templateId` (keep the existing `refreshKey` bump for the right panel).
- [ ] 4.3 Compose the three columns: left panel tabbed `Editor` (`<CvSectionForm bind:doc … embedded />`) / `Chat` (`<AssistantChat …/>`); centre `<CvHtmlPreview {doc} {templateId} />` + zoom control + Download PDF (`cvPdfUrl(cvId)`); right `<ArtifactPanel …/>`.
- [ ] 4.4 Two resizable side panels using `clampWidth` (two widths in `$state`, pointer-capture splitters), centre `flex-1`.

## 5. Verify

- [ ] 5.1 `npm run check` (svelte-check) + `vitest` green; `npm run build` succeeds.
- [ ] 5.2 Drive end-to-end (visual verify): fit CTA → `/tailor/[slug]` renders three columns; Editor↔Chat tab switch; typing a field re-renders the centre preview instantly; Templates/JD/Verdict tabs; template select changes the PDF; Download PDF fetches; agent edit turn refreshes the preview; side panels resize+clamp.
- [ ] 5.3 `/my/cvs` regression re-confirmed (send/edit/save unchanged).
