import { describe, it, expect } from 'vitest';
import { buildActivityChart, formatCount, pickTickIndices, type ActivityBar } from './activityChart';
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

  it('centres each bar-pair at the seam between its two bars', () => {
    const m = buildActivityChart([pt('2026-01-01', 4, 4)]);
    const bar = requireBar(m.bars, 0);
    // The slot centre is where the added bar ends and the removed bar begins.
    expect(bar.centerX).toBeCloseTo(bar.addedX + m.barW);
    expect(bar.centerX).toBeCloseTo(bar.removedX);
  });
});

describe('formatCount', () => {
  it('leaves small numbers as-is', () => {
    expect(formatCount(0)).toBe('0');
    expect(formatCount(842)).toBe('842');
  });
  it('abbreviates thousands and millions', () => {
    expect(formatCount(697191)).toBe('697K');
    expect(formatCount(3354251)).toBe('3.4M');
    expect(formatCount(3400)).toBe('3.4K');
  });
});

describe('pickTickIndices', () => {
  it('labels every index when there are few', () => {
    expect(pickTickIndices(3)).toEqual([0, 1, 2]);
  });
  it('always includes the first and last index', () => {
    const ticks = pickTickIndices(90);
    expect(ticks[0]).toBe(0);
    expect(ticks[ticks.length - 1]).toBe(89);
  });
  it('thins a long series to a readable number of ticks', () => {
    const ticks = pickTickIndices(90);
    expect(ticks.length).toBeLessThanOrEqual(12);
    // strictly increasing, in range
    for (let i = 1; i < ticks.length; i++) {
      expect(ticks[i]!).toBeGreaterThan(ticks[i - 1]!);
      expect(ticks[i]!).toBeLessThan(90);
    }
  });
  it('returns nothing for an empty series', () => {
    expect(pickTickIndices(0)).toEqual([]);
  });
});
