// Shared formatting for a skill's percent-change trend — used by both the
// compact card grid (MarketPulseView) and the per-skill detail page, so the
// sign/rounding rule lives in exactly one place.

/** Sign, then magnitude — a true minus sign (not a hyphen), matching
 *  PlanView's signed-amount formatting. `rounded` must already be rounded to a whole
 *  percent; a fractional point is noise here. */
export function fmtPct(rounded: number): string {
  return rounded > 0 ? `+${rounded}%` : rounded < 0 ? `−${Math.abs(rounded)}%` : '0%';
}

/** The up/good, down/bad semantic a trend's accent color follows — shared by
 *  the sparkline's accent dot and the detail chart's line/marker. Takes the
 *  ROUNDED percent (matching what fmtPct prints): branching on the raw value
 *  would let a sub-0.5% change pick an up/down color while the printed badge
 *  still reads "0%". */
export function trendDotClass(rounded: number | null): string {
  if (rounded === null || rounded === 0) return 'fill-foreground';
  return rounded > 0 ? 'fill-emerald-500' : 'fill-rose-500';
}
