import { describe, it, expect } from 'vitest';
import { buildSparkline } from './skillPulseSparkline';
import type { SkillPulsePoint } from './api';

const pt = (week_start: string, open_count: number): SkillPulsePoint => ({ week_start, open_count });

// Parses one "x,y" entry from the `points` SVG attribute string — the tests only
// ever feed it a value `buildSparkline` itself produced, so the shape is known.
function parsePoint(raw: string): { x: number; y: number } {
  const [x, y] = raw.split(',').map(Number);
  return { x: x ?? 0, y: y ?? 0 };
}

describe('buildSparkline', () => {
  it('draws nothing for an empty series', () => {
    const m = buildSparkline([]);
    expect(m.points).toBe('');
    expect(m.width).toBeGreaterThan(0);
    expect(m.height).toBeGreaterThan(0);
  });

  it('draws only the accent dot for a single-point series', () => {
    const m = buildSparkline([pt('2026-08-10', 40)]);
    expect(m.points).toBe('');
    expect(m.lastX).toBeGreaterThan(0);
    expect(m.lastY).toBeGreaterThan(0);
  });

  it('centers a flat series on the frame midline rather than the baseline', () => {
    // A flat high-level series (e.g. steady demand of 50) must not visually read as
    // "near zero" just because nothing changed — see the divide-by-zero fallback.
    const m = buildSparkline([pt('2026-08-03', 50), pt('2026-08-10', 50)]);
    expect(Number.isFinite(m.lastY)).toBe(true);
    const points = m.points.split(' ').map(parsePoint);
    expect(points).toHaveLength(2);
    for (const p of points) expect(p.y).toBeCloseTo(16); // HEIGHT / 2
  });

  it('clamps the min to the bottom edge and the max to the top edge, inset by the y padding', () => {
    const m = buildSparkline([pt('2026-07-27', 10), pt('2026-08-03', 100), pt('2026-08-10', 55)]);
    const points = m.points.split(' ').map(parsePoint);
    expect(points).toHaveLength(3);
    expect(points[0]!.y).toBeCloseTo(29); // the min (10) → HEIGHT(32) - PAD_Y(3)
    expect(points[1]!.y).toBeCloseTo(3); // the max (100) → PAD_Y(3)
  });

  it("the accent dot sits at the last point's own computed x,y, not a default", () => {
    const m = buildSparkline([pt('2026-07-27', 10), pt('2026-08-03', 100), pt('2026-08-10', 55)]);
    // width=120 over 3 points → stepX=60, so the 3rd point's x is exactly the frame edge.
    expect(m.lastX).toBeCloseTo(120);
    // 55 is the exact midpoint of [10, 100] → its y is the frame midline.
    expect(m.lastY).toBeCloseTo(16);
  });
});
