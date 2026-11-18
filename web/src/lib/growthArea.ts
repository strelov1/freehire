// Pure geometry for GrowthArea: where each point of a dated series sits on the x-axis.

const DAY_MS = 86_400_000;

/** The x of each point, spaced by the UTC days between their dates rather than by their
 *  position in the array, so a missing day reads as a gap and not as one day's step. A
 *  single point, or a series on one day, sits in the middle. Dates are ascending
 *  YYYY-MM-DD. */
export function dateXs(dates: string[], width: number, pad: number): number[] {
  const t = dates.map((d) => Date.parse(`${d}T00:00:00Z`) / DAY_MS);
  const first = t[0] ?? 0;
  const span = (t[t.length - 1] ?? 0) - first;
  return t.map((v) => (span <= 0 ? width / 2 : pad + ((v - first) / span) * (width - 2 * pad)));
}
