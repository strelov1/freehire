import { describe, it, expect } from 'vitest';
import { buildActivityChart, type ActivityBar } from './activityChart';
import type { ActivityPoint } from './types';

const pt = (period: string, added: number, removed: number): ActivityPoint => ({
  period,
  added,
  removed,
});

// requireBar asserts a bar exists at index i and returns it narrowed, so the
// tests read straight-line under noUncheckedIndexedAccess.
function requireBar(bars: ActivityBar[], i: number): ActivityBar {
  const bar = bars[i];
  if (!bar) throw new Error(`expected a bar at index ${i}`);
  return bar;
}

describe('buildActivityChart', () => {
  it('returns a drawable but empty frame for no points', () => {
    const m = buildActivityChart([]);
    expect(m.bars).toEqual([]);
    expect(m.max).toBe(1); // never zero, so downstream scaling can't divide by zero
    expect(m.height).toBeGreaterThan(0);
    expect(m.baselineY).toBeGreaterThan(0);
  });

  it('scales bar heights proportionally to the largest single count', () => {
    const m = buildActivityChart([pt('2026-01-01', 10, 0), pt('2026-01-02', 5, 0)]);
    expect(m.max).toBe(10);
    expect(m.bars).toHaveLength(2);
    const first = requireBar(m.bars, 0);
    const second = requireBar(m.bars, 1);
    // 10 vs 5 → the first added bar is exactly twice the height of the second.
    expect(first.addedH).toBeCloseTo(2 * second.addedH);
    // A zero count is a zero-height bar.
    expect(first.removedH).toBe(0);
  });

  it('shares one axis across added and removed', () => {
    const bar = requireBar(buildActivityChart([pt('2026-01-01', 10, 5)]).bars, 0);
    // removed is half of added, so its bar is half the height on the shared scale.
    expect(bar.removedH).toBeCloseTo(bar.addedH / 2);
  });

  it('grows bars up from the baseline', () => {
    const m = buildActivityChart([pt('2026-01-01', 8, 3)]);
    const bar = requireBar(m.bars, 0);
    expect(bar.addedY).toBeCloseTo(m.baselineY - bar.addedH);
    expect(bar.removedY).toBeCloseTo(m.baselineY - bar.removedH);
  });

  it('places the removed bar to the right of the added bar without overlap', () => {
    const m = buildActivityChart([pt('2026-01-01', 4, 4)]);
    const bar = requireBar(m.bars, 0);
    expect(bar.removedX).toBeGreaterThanOrEqual(bar.addedX + m.barW);
  });
});
