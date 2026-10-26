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
