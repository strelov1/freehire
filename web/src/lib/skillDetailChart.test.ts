import { describe, it, expect } from 'vitest';
import { buildSkillDetailChart } from './skillDetailChart';
import { must } from './utils';
import type { SkillPulsePoint } from './api';

const pt = (week_start: string, open_count: number): SkillPulsePoint => ({ week_start, open_count });

describe('buildSkillDetailChart', () => {
  it('returns a drawable but empty frame for no points', () => {
    const m = buildSkillDetailChart([]);
    expect(m.points).toEqual([]);
    expect(m.width).toBeGreaterThan(0);
    expect(m.height).toBeGreaterThan(0);
    expect(m.max).toBe(1); // never zero, so downstream scaling can't divide by zero
  });

  it('centers a single point in the frame', () => {
    const m = buildSkillDetailChart([pt('2026-08-10', 40)]);
    expect(m.points).toHaveLength(1);
    const p0 = must(m.points[0]);
    expect(p0.x).toBeCloseTo(m.width / 2);
    expect(p0.openCount).toBe(40);
  });

  it('centers a flat series on the midline rather than the baseline', () => {
    const m = buildSkillDetailChart([pt('2026-08-03', 50), pt('2026-08-10', 50)]);
    for (const p of m.points) expect(p.y).toBeCloseTo(m.topY + (m.baselineY - m.topY) / 2);
  });

  it('places the max at the top and the min at the baseline', () => {
    const m = buildSkillDetailChart([pt('2026-07-27', 10), pt('2026-08-03', 100), pt('2026-08-10', 55)]);
    const p0 = must(m.points[0]);
    const p1 = must(m.points[1]);
    expect(p1.y).toBeCloseTo(m.topY); // 100 = max
    expect(p0.y).toBeCloseTo(m.baselineY); // 10 = min
    expect(m.max).toBe(100);
  });

  it('carries each point\'s own week and count for the hover tooltip', () => {
    const m = buildSkillDetailChart([pt('2026-08-03', 10), pt('2026-08-10', 20)]);
    expect(m.points.map((p) => [p.weekStart, p.openCount])).toEqual([
      ['2026-08-03', 10],
      ['2026-08-10', 20],
    ]);
  });
});
