## Why

The CV tailor workspace shows the live preview as one endless white column with no
page boundaries, so a user building a two-page résumé cannot see where page 1 ends and
page 2 begins, nor tell whether their content overflows. The PDF already paginates (Typst
sets `paper: "a4"`), but the preview does not mirror it — and page margins are hardcoded
per template, so users cannot adjust how tightly content packs to fit one or two pages.

## What Changes

- Add configurable page margins (top / right / bottom / left, in inches) to a CV, applied
  identically to the live HTML preview and the generated PDF.
- The live preview renders as discrete A4 sheets (page 1, page 2, …) with block-aware page
  breaks instead of one continuous column, mirroring how Typst paginates the PDF.
- Add a "Margins" settings panel to the workspace editor (left sidebar) with per-side
  steppers (0.05″ step, 0.25″–1.5″ range) that update the preview live and persist via autosave.
- Typst templates read margins from the document (default 0.5″ per side) instead of hardcoding them.

## Capabilities

### New Capabilities
<!-- none: behaviour extends the existing cv-builder and tailor-workspace specs -->

### Modified Capabilities
- `cv-builder`: CV documents carry per-side page margins; the sanitizer clamps and defaults
  them; the PDF renderer applies them via the Typst templates.
- `tailor-workspace`: the centre preview paginates into A4 sheets that honour the margins;
  the editor gains a margins settings panel that drives preview and PDF.

## Impact

- Backend: `internal/cv/cv.go` (Document + Margins + Sanitize), all four
  `internal/cv/templates/*.typ` (read margins), generated `web/src/lib/generated/contracts.ts`.
- Frontend: `web/src/lib/tailor/CvHtmlPreview.svelte` (pagination + margins), the workspace
  editor (`web/src/routes/tailor/[slug]/+page.svelte` / `CvSectionForm`) for the settings panel,
  `web/src/lib/cv.ts` (`toEditable` carries margins).
- No DB migration: margins live inside the existing `document` JSON blob.
- Existing CVs render with the 0.5″ default (a small shift from the current per-template margins).
