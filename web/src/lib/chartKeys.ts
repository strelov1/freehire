export function nextChartIndex(key: string, current: number | null, length: number): number | null {
  if (length <= 0) return null;
  if (key === 'Home') return 0;
  if (key === 'End') return length - 1;
  if (key === 'ArrowRight') return current === null ? 0 : Math.min(length - 1, current + 1);
  if (key === 'ArrowLeft') return current === null ? length - 1 : Math.max(0, current - 1);
  return null;
}

export function chartTipPosition(el: HTMLElement, fractionX: number): { x: number; y: number } {
  const rect = el.getBoundingClientRect();
  return { x: fractionX * rect.width, y: Math.max(24, rect.height * 0.35) };
}

/** The index of the point whose x is nearest `x` (both in viewBox units), or null for no
 *  points. A linear scan: the charts that use it draw at most a few dozen points, and
 *  nearest-by-x stays right however the chart spaces them — gaps included. */
export function nearestIndexByX(points: readonly { x: number }[], x: number): number | null {
  let nearest: number | null = null;
  let nearestDist = Infinity;
  points.forEach((p, i) => {
    const d = Math.abs(p.x - x);
    if (d < nearestDist) {
      nearestDist = d;
      nearest = i;
    }
  });
  return nearest;
}
