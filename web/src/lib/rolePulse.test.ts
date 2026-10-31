import { describe, expect, it } from 'vitest';
import type { InsightRolePulse } from './api';
import { rolePulseFigures } from './rolePulse';

function pulse(over: Partial<InsightRolePulse> = {}): InsightRolePulse {
  return {
    category: 'backend',
    open_count: 1000,
    open_count_prev: 900,
    competition: {
      views: 4500,
      views_prev: 3000,
      exposure_days: 30000,
      exposure_days_prev: 30000,
      views_through: '2026-10-29',
    },
    time_to_close: { closed: 200, closed_prev: 180, median_days: 27.4, median_days_prev: 31.2 },
    inflow: { count: 1200 },
    ...over,
  };
}

type Figure = ReturnType<typeof rolePulseFigures>[number];

function byKey(p: InsightRolePulse): Record<Figure['key'], Figure> {
  const [competition, time_to_close, inflow] = rolePulseFigures(p);
  if (!competition || !time_to_close || !inflow) throw new Error('expected three figures');
  return { competition, time_to_close, inflow };
}

describe('rolePulseFigures', () => {
  it('keeps a fixed order', () => {
    expect(rolePulseFigures(pulse()).map((f) => f.key)).toEqual([
      'competition',
      'time_to_close',
      'inflow',
    ]);
  });

  it('scales views to a month per open posting', () => {
    const f = byKey(pulse()).competition;
    // 4500 views over 30000 posting-days = 4.5 a month; 3000 → 3.
    expect(f.value).toBe('4.5');
    expect(f.previous).toBe('3');
    expect(f.direction).toBe('up');
  });

  it('hides competition below the exposure or view floor', () => {
    const thin = byKey(
      pulse({
        competition: { views: 40, views_prev: 30, exposure_days: 5000, exposure_days_prev: 5000, views_through: null },
      }),
    ).competition;
    expect(thin.value).toBeNull();
    expect(thin.direction).toBeNull();
    const short = byKey(
      pulse({
        competition: { views: 4500, views_prev: 3000, exposure_days: 200, exposure_days_prev: 30000, views_through: null },
      }),
    ).competition;
    expect(short.value).toBeNull();
  });

  it('shows no direction when only the earlier window is thin', () => {
    const f = byKey(
      pulse({ time_to_close: { closed: 200, closed_prev: 4, median_days: 27, median_days_prev: 90 } }),
    ).time_to_close;
    expect(f.value).toBe('27 days');
    expect(f.previous).toBeNull();
    expect(f.direction).toBeNull();
  });

  it('hides the median below the close floor', () => {
    const f = byKey(
      pulse({ time_to_close: { closed: 29, closed_prev: 180, median_days: 27, median_days_prev: 31 } }),
    ).time_to_close;
    expect(f.value).toBeNull();
    expect(f.detail).not.toContain('29');
  });

  it('reads a change under 5% as flat', () => {
    const f = byKey(
      pulse({ time_to_close: { closed: 200, closed_prev: 180, median_days: 30, median_days_prev: 29 } }),
    ).time_to_close;
    expect(f.direction).toBe('flat');
    expect(byKey(pulse()).time_to_close.direction).toBe('down');
  });

  it('shows the inflow count with no comparison', () => {
    const f = byKey(pulse({ inflow: { count: 12345 } })).inflow;
    expect(f.value).toBe('12,345');
    expect(f.previous).toBeNull();
    expect(f.direction).toBeNull();
  });
});
