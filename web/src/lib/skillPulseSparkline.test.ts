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

  it('places the highest count nearest the top (smallest y) of the frame', () => {
    const m = buildSparkline([pt('2026-08-03', 10), pt('2026-08-10', 100)]);
    const parsed = m.points.split(' ').map(parsePoint);
    const first = parsed[0] ?? { x: 0, y: 0 };
    const second = parsed[1] ?? { x: 0, y: 0 };
    expect(second.y).toBeLessThan(first.y);
  });

  it("the last point's coordinates match the series' final value", () => {
    const m = buildSparkline([pt('2026-08-03', 10), pt('2026-08-10', 100)]);
    const points = m.points.split(' ');
    const last = parsePoint(points.at(-1) ?? '');
    expect(m.lastX).toBeCloseTo(last.x);
    expect(m.lastY).toBeCloseTo(last.y);
  });
});
