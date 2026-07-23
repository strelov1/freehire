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
    const page = pages[pages.length - 1]!;
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
