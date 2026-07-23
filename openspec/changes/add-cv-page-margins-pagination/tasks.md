## 1. Backend: margins in the document model

- [x] 1.1 Add a `Margins` type and a `Margins` field (json `margins`) to `cv.Document` in `internal/cv/cv.go`
- [x] 1.2 In `Sanitize`, clamp each margin to 0.25–1.5″ and default a zero/unset side to 0.5″; add unit tests covering default, clamp-high, clamp-low, and pass-through
- [x] 1.3 Verify tailoring/patching preserves margins (a `Tailor` copy inherits them; a field-level patch leaves them intact) — add/extend a test if not already covered

## 2. Backend: PDF rendering honours margins

- [x] 2.1 Update all four `internal/cv/templates/*.typ` to read `cv.margins` (per-side, default 0.5) and set `#set page(margin: …)` from it
- [x] 2.2 Add a renderer test that compiles a template with custom margins and asserts a non-empty PDF is produced (guard behind the existing typst-available check)

## 3. Contract + frontend model plumbing

- [x] 3.1 Regenerate `web/src/lib/generated/contracts.ts` from the Go type so `Document` carries `margins`
- [x] 3.2 Ensure `toEditable` in `web/src/lib/cv.ts` carries the margins field through (default 0.5 per side when absent); add a unit test

## 4. Frontend: pagination logic (pure, unit-tested)

- [x] 4.1 Add a pure `paginateBlocks(blockHeights: number[], pageBodyHeight: number): number[][]` helper (greedy block packing; a block taller than a page gets its own page) in a testable module
- [x] 4.2 Unit-test `paginateBlocks`: single page, exact-fit boundary, overflow to page 2, and an oversized single block

## 5. Frontend: paginated A4 preview

- [x] 5.1 In `CvHtmlPreview.svelte`, derive page geometry from `doc.margins` (content width, page body height) and apply margins as sheet padding
- [x] 5.2 Measure section-block heights (ResizeObserver / `$effect`) and render N stacked A4 sheets via `paginateBlocks`, with an inter-sheet gap; keep `zoom` scaling the whole stack
- [x] 5.3 Handle the sidebar template: paginate the main column, keep the narrow sidebar column on sheet 1
- [x] 5.4 Visually verify (headless Chrome throwaway route) 1-page, 2-page, and margin-change cases across templates

## 6. Frontend: margins settings panel

- [x] 6.1 Add a "Margins (in inches)" settings section to the workspace editor with four steppers (top/right/bottom/left), 0.05″ step, 0.25–1.5″ range, two-decimal display, bound to `doc.margins`
- [x] 6.2 Confirm a margin change re-renders the preview live, is autosaved, and cache-busts the PDF (`pdfVersion`)

## 7. Verify

- [x] 7.1 `go build ./... && go vet ./... && go test ./...`
- [x] 7.2 Web: eslint + build + relevant vitest pass
- [x] 7.3 End-to-end sanity: adjust margins in the workspace, confirm preview repaginates and Download PDF matches
