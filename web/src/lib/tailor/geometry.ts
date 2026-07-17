/** Clamp a pixel width to [min, max], rounded to whole pixels. Used by the artifact-panel
 *  splitter so a drag can't collapse or overflow the panel. */
export function clampWidth(px: number, min: number, max: number): number {
  return Math.round(Math.max(min, Math.min(max, px)));
}
