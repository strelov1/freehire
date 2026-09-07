import { describe, it, expect } from 'vitest';
import { buildSignupsChart, type SignupsBar } from './signupsChart';
import type { UserGrowthPoint } from './types';

const pt = (date: string, total: number, n: number): UserGrowthPoint => ({
  date,
  total,
  new: n,
});

function requireBar(bars: SignupsBar[], i: number): SignupsBar {
  const bar = bars[i];
  if (!bar) throw new Error(`expected a bar at index ${i}`);
  return bar;
}

describe('buildSignupsChart', () => {
  it('returns a drawable but empty frame for no points', () => {
    const m = buildSignupsChart([]);
    expect(m.bars).toEqual([]);
    expect(m.max).toBe(1); // never zero, so downstream scaling can't divide by zero
    expect(m.height).toBeGreaterThan(0);
    expect(m.baselineY).toBeGreaterThan(0);
  });

  it('scales bar heights proportionally to the largest single day', () => {
    const m = buildSignupsChart([pt('2026-01-01', 10, 10), pt('2026-01-02', 15, 5)]);
    expect(m.max).toBe(10);
    expect(m.bars).toHaveLength(2);
    const first = requireBar(m.bars, 0);
    const second = requireBar(m.bars, 1);
    // 10 vs 5 → the first bar is exactly twice the height of the second.
    expect(first.h).toBeCloseTo(2 * second.h);
  });

  it('renders a zero-signup day as a zero-height bar', () => {
    const m = buildSignupsChart([pt('2026-01-01', 5, 5), pt('2026-01-02', 5, 0)]);
    const bar = requireBar(m.bars, 1);
    expect(bar.h).toBe(0);
  });

  it('grows bars up from the baseline', () => {
    const m = buildSignupsChart([pt('2026-01-01', 8, 8)]);
    const bar = requireBar(m.bars, 0);
    expect(bar.y).toBeCloseTo(m.baselineY - bar.h);
  });
});
