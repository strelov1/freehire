## Context

The CV tailor workspace (`web/src/routes/tailor/[slug]/+page.svelte`) owns one in-memory
`Document` shared by the structured editor and the centre preview. `CvHtmlPreview.svelte` renders
that document as a single fixed-width (794px ≈ A4 @96dpi) white column that grows unbounded — no
page boundaries. The real PDF is produced server-side by `internal/cv/renderer.go` (Typst CLI),
which reads the document as `data.json`; each of the four `templates/*.typ` hardcodes
`#set page(margin: …)`. Typst paginates automatically, so the PDF is already multi-page; the
preview is the mismatch, and margins are not user-adjustable.

The document is stored as a single JSON blob (no per-field columns), autosave persists the whole
`{title, templateId, doc}` snapshot, the tailoring agent edits via field-level `PatchCV` (never a
whole-document replace), and `cvStore.Tailor` copies the base document into the tailored copy.

## Goals / Non-Goals

**Goals:**
- Per-side page margins (inches) on a CV, applied identically to preview and PDF.
- Preview rendered as discrete A4 sheets with block-aware page breaks.
- A margins settings panel in the workspace editor.

**Non-Goals:**
- Pixel-perfect parity between HTML preview breaks and Typst's break algorithm — the preview is an
  honest approximation; the PDF is the source of truth.
- Splitting a single section that is itself taller than a page (rare); it spills on its own sheet.
- Per-template or per-page independent margins; margins are one set per document.
- A DB migration or new column.

## Decisions

**Margins live inside `cv.Document` (not sibling metadata).** The Typst renderer only receives the
document, autosave already persists the whole document, tailoring copies it, and patches never
touch it. Putting `Margins` in the document means zero new plumbing — no migration, no renderer
signature change, no separate fetch in the workspace. Alternative (a `cvs.margins` jsonb column +
render query param) was rejected as strictly more code for no benefit at this stage.

**Inches as the single canonical unit.** HTML converts exactly (`px = inches × 96`); Typst accepts
inch units natively (`0.5 * 1in`). One stored float feeds both renderers with no unit drift.

**Sanitizer clamps to 0.25–1.5″, defaults 0 → 0.5″.** Keeps persisted CVs sane and lets existing
CVs (no margin field) render with a sensible default. Default 0.5″ per side (matches the settings
UI); this slightly shifts current output (was ~0.55″ x / ~0.43″ y) — acceptable in beta.

**Preview pagination = measure-then-distribute at block level.** Render the section blocks, measure
each block's offset/height (unscaled, pre-`zoom`), greedily pack blocks into page containers whose
body height = `1123 − (top+bottom)×96` px, and render N stacked white A4 sheets (each 794×1123px,
padded by the margins) with a gap between them. `zoom` scales the whole stack. This avoids DOM
slicing (no broken layout) while looking like real sheets. The greedy pack function is the one piece
of genuine logic and is unit-testable as a pure function over `[{height}]`.

**Sidebar template: paginate the main column, sidebar stays on sheet 1.** The two-column grid can't
be block-split symmetrically; the narrow column (contact/skills) is short by nature, so it renders on
page 1 while the main column paginates. Documented as an approximation.

**Steppers: 0.05″ step, 0.25–1.5″ range, two-decimal display.** Matches résumé-editor granularity.

## Risks / Trade-offs

- **Measurement timing / reflow** → measure in an `$effect` after render with a `ResizeObserver` on
  the content, recomputing page breaks when content or margins change; debounce is unnecessary at
  this scale.
- **A block taller than one page body** → it renders on its own sheet and may visually overflow; we
  accept this (rare) and rely on the "PDF is truth" framing rather than intra-block splitting.
- **Preview breaks won't exactly match Typst** → intentional; the preview communicates page count
  and approximate breaks, the Download PDF is authoritative.
- **Default margin shift changes existing CVs' look** → beta-only feature, low blast radius; the new
  default is a clean 0.5″ users can adjust.
- **Generated contract drift** → regenerate `contracts.ts` from the Go type; `toEditable` must carry
  the margins field through.

## Migration Plan

No schema migration. Deploy is code-only. Existing CVs have no `margins` field → the sanitizer/
templates default every side to 0.5″. Rollback is a straight revert (documents keep the harmless
extra `margins` key, ignored by the old code).

## Open Questions

None — margin unit, bounds, default, and sidebar behaviour were resolved during brainstorming.
