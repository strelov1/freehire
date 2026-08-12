import type { Margins } from '$lib/generated/contracts';
import { must } from '../utils';

/** Clamp a pixel width to [min, max], rounded to whole pixels. Used by the artifact-panel
 *  splitter so a drag can't collapse or overflow the panel. */
export function clampWidth(px: number, min: number, max: number): number {
  return Math.round(Math.max(min, Math.min(max, px)));
}

/** Page-margin stepper bounds and increment, in inches — mirrors the backend's clamp band. */
export const MARGIN_MIN = 0.25;
export const MARGIN_MAX = 1.5;
export const MARGIN_STEP = 0.05;

/** Apply one stepper increment to a margin: add delta, clamp to [MARGIN_MIN, MARGIN_MAX], and
 *  round to two decimals so repeated 0.05″ steps never accumulate float drift. */
export function stepMargin(value: number, delta: number): number {
  const next = Math.round((value + delta) * 100) / 100;
  return Math.min(MARGIN_MAX, Math.max(MARGIN_MIN, next));
}

/** The two axes the margin settings link by default. Uniform margins are the common case, and
 *  four steppers abreast do not fit the workspace panel at its narrow end. */
export type MarginAxis = 'sides' | 'ends';
const AXIS_SIDES: Record<MarginAxis, ['left' | 'top', 'right' | 'bottom']> = {
  sides: ['left', 'right'],
  ends: ['top', 'bottom'],
};

/** The shared value of an axis, or null when its two sides differ. Null is what lets the linked
 *  view admit an asymmetry instead of showing one side and overwriting the other on the next step. */
export function axisValue(m: Margins, axis: MarginAxis): number | null {
  const [a, b] = AXIS_SIDES[axis];
  return m[a] === m[b] ? m[a] : null;
}

/** Step both sides of an axis, each clamped on its own. An asymmetric axis stays asymmetric:
 *  the user opened the per-side controls to make it so, and a nudge here must not level it. */
export function stepAxis(m: Margins, axis: MarginAxis, delta: number): Margins {
  const [a, b] = AXIS_SIDES[axis];
  return { ...m, [a]: stepMargin(m[a], delta), [b]: stepMargin(m[b], delta) };
}

/** Font-size stepper bounds and increment, in points — mirrors the backend's clamp band. */
export const FONT_SIZE_MIN = 8.5;
export const FONT_SIZE_MAX = 12;
export const FONT_SIZE_STEP = 0.5;

/** The base type size every Typst template sets when the document names none. A zero size in
 *  the document means "use this", so the stepper starts from it rather than from zero. */
export const TEMPLATE_FONT_SIZE_PT = 9.5;

/** The unset-style fallback in CSS px: TEMPLATE_FONT_SIZE_PT at 96dpi (1pt = 4/3px), kept
 *  fractional for the same reason `stepFontSize`'s own conversion is — this used to be a
 *  rounded 13px, which is 9.75pt-equivalent, not the template's actual 9.5pt. A ~2.6% oversized
 *  baseline is invisible on one line and compounds on a dense CV like every other constant here. */
export const PREVIEW_FONT_SIZE_PX = (TEMPLATE_FONT_SIZE_PT * 4) / 3;

/** Measured, not guessed: a classic-ats PDF's text layer (PyMuPDF span bboxes) advances a
 *  consistent 11.0pt between two lines of the same paragraph at the template's default 9.5pt /
 *  0.5em-leading — a 11/9.5 line-height multiplier, not the 1.375 this used to read. That
 *  earlier constant overstated every line by ~19%, which a short CV absorbs invisibly but a
 *  dense one (many bulleted roles) compounds into a page's worth of premature overflow: a real
 *  PDF that fits five roles on page one measured a preview that pushed the fourth onto page two. */
export const PREVIEW_LINE_HEIGHT = 11 / 9.5;

/** CSS `line-height` ≈ Typst `leading` + this. Typst's leading is the gap BETWEEN lines; CSS
 *  line-height is the whole line box, so they differ by roughly the glyphs' own height. The
 *  constant is calibrated, not derived: it is what makes the templates' default 0.5em leading
 *  land on PREVIEW_LINE_HEIGHT above. Change it and the preview starts paginating in a
 *  different metric from the PDF, silently. */
const LEADING_TO_LINE_HEIGHT = PREVIEW_LINE_HEIGHT - 0.5;

/** Apply one stepper increment to a base font size in points. An unset (zero) size steps from
 *  the template's own base, so the first nudge upward reads as 9.5 → 10 rather than collapsing
 *  to the minimum. Rounded to one decimal so repeated 0.5pt steps never drift. */
export function stepFontSize(value: number, delta: number): number {
  const from = value > 0 ? value : TEMPLATE_FONT_SIZE_PT;
  const next = Math.round((from + delta) * 10) / 10;
  return Math.min(FONT_SIZE_MAX, Math.max(FONT_SIZE_MIN, next));
}

/** The typography the live preview renders (and measures) with, resolved from the document's
 *  style block. Every unset value falls back to what the preview used before the style block
 *  existed; `cssStack` is passed straight through as `fontFamily` — the caller resolves it,
 *  whether that's the candidate's chosen font or the active template's own default face, so an
 *  unset font_family still measures against the face the PDF will actually use rather than
 *  whatever generic serif/sans the browser's OS happens to default to.
 *
 *  Pure so the calibration above is testable: the component only spreads the result onto both
 *  its visible sheets and its hidden measurement layer. */
export function previewTypography(
  style: { font_family?: string; font_size?: number; line_height?: number },
  cssStack: string,
): { fontSizePx: number; lineHeight: number; fontFamily: string } {
  const pt = style.font_size ?? 0;
  const leading = style.line_height ?? 0;
  return {
    // 96dpi: 1pt = 4/3 px, kept fractional. Rounding to whole pixels collapsed 9.5pt and 10pt
    // onto 13px (and 11 with 11.5 onto 15), so two of the eight stepper positions moved the PDF
    // and left the preview still — in a feature whose whole point is that they agree.
    fontSizePx: pt > 0 ? (pt * 4) / 3 : PREVIEW_FONT_SIZE_PX,
    lineHeight: leading > 0 ? leading + LEADING_TO_LINE_HEIGHT : PREVIEW_LINE_HEIGHT,
    fontFamily: cssStack,
  };
}

/** Distribute measured section-block heights across A4 page bodies for the live preview.
 *  Greedy block-level pagination: a section is never split across the inter-page gap; when
 *  the next block would overflow the current page body, it starts the next page. A block
 *  taller than a whole page gets its own page rather than looping forever. Empty input
 *  yields one empty page so the preview always shows at least one sheet.
 *
 *  @param blockHeights unscaled pixel height of each top-level section, in document order
 *  @param pageBodyHeight usable pixel height of one A4 sheet (page height minus top+bottom margins)
 *  @returns pages, each an array of block indices assigned to that page
 */
export function paginateBlocks(blockHeights: number[], pageBodyHeight: number): number[][] {
  const pages: number[][] = [[]];
  let used = 0;
  blockHeights.forEach((h, i) => {
    const page = must(pages[pages.length - 1]);
    // Break only when the current page already carries something, so a block taller than a
    // whole page lands on its own page instead of looping forever.
    if (page.length > 0 && used + h > pageBodyHeight) {
      pages.push([i]);
      used = h;
    } else {
      page.push(i);
      used += h;
    }
  });
  return pages;
}
