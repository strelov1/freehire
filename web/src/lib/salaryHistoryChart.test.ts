import { describe, expect, it } from 'vitest';
import type { InsightSalaryMonth } from './api';
import { buildSalaryChart, historySeries } from './salaryHistoryChart';

const row = (month: string, p50: number, over: Partial<InsightSalaryMonth> = {}): InsightSalaryMonth => ({
  currency: 'USD',
  period: 'year',
  month,
  sample_size: 40,
  p25: p50 - 10,
  p50,
  p75: p50 + 10,
  ...over,
});

describe('historySeries', () => {
  it('groups by currency and period, largest total sample first', () => {
    const series = historySeries([
      row('2026-07', 100, { currency: 'EUR', sample_size: 90 }),
      row('2026-08', 100, { currency: 'EUR', sample_size: 90 }),
      row('2026-09', 100, { currency: 'EUR', sample_size: 90 }),
      row('2026-07', 100),
      row('2026-08', 100),
      row('2026-09', 100),
    ]);
    expect(series.map((s) => `${s.currency}/${s.period}`)).toEqual(['EUR/year', 'USD/year']);
  });

  it('drops a series with fewer than three months', () => {
    const series = historySeries([row('2026-08', 1), row('2026-09', 1), row('2026-09', 1, { period: 'month' })]);
    expect(series).toEqual([]);
  });

  it('orders a series by month whatever order the rows came in', () => {
    const [s] = historySeries([row('2026-09', 3), row('2026-07', 1), row('2026-08', 2)]);
    expect(s?.months.map((m) => m.month)).toEqual(['2026-07', '2026-08', '2026-09']);
  });
});

describe('buildSalaryChart', () => {
  const series = (rows: InsightSalaryMonth[]) => {
    const [s] = historySeries(rows);
    if (!s) throw new Error('no series');
    return s;
  };

  it('gives a missing month a slot but no point, and splits the line there', () => {
    const model = buildSalaryChart(series([row('2026-01', 100), row('2026-02', 110), row('2026-04', 120), row('2026-05', 130)]), '2026-10');
    expect(model.slots).toEqual(['2026-01', '2026-02', '2026-03', '2026-04', '2026-05']);
    expect(model.segments.map((seg) => seg.map((p) => p.month))).toEqual([
      ['2026-01', '2026-02'],
      ['2026-04', '2026-05'],
    ]);
  });

  it('spaces points by slot, so the gap keeps its width', () => {
    const model = buildSalaryChart(series([row('2026-01', 100), row('2026-02', 110), row('2026-04', 120)]), '2026-10');
    const [a, b, c] = model.points;
    expect((c?.x ?? 0) - (b?.x ?? 0)).toBeCloseTo(2 * ((b?.x ?? 0) - (a?.x ?? 0)));
  });

  it('marks the current month as partial', () => {
    const model = buildSalaryChart(series([row('2026-08', 100), row('2026-09', 110), row('2026-10', 120)]), '2026-10');
    expect(model.points.map((p) => p.partial)).toEqual([false, false, true]);
  });

  it('scales the band to the plot: highest p75 on top, lowest p25 on the baseline', () => {
    const model = buildSalaryChart(series([row('2026-08', 100), row('2026-09', 200), row('2026-10', 150)]), '2026-12');
    const ys = model.points.flatMap((p) => [p.yHigh, p.yLow]);
    expect(Math.min(...ys)).toBeCloseTo(model.topY);
    expect(Math.max(...ys)).toBeCloseTo(model.baselineY);
    expect(model.min).toBe(90);
    expect(model.max).toBe(210);
  });

  it('draws a flat series on the midline instead of dividing by zero', () => {
    const flat = series([row('2026-08', 100, { p25: 100, p75: 100 }), row('2026-09', 100, { p25: 100, p75: 100 }), row('2026-10', 100, { p25: 100, p75: 100 })]);
    const model = buildSalaryChart(flat, '2026-12');
    for (const p of model.points) expect(p.yMid).toBeCloseTo((model.topY + model.baselineY) / 2);
  });

  it('flags a month whose sample jumped at least threefold over the month before', () => {
    const model = buildSalaryChart(
      series([
        row('2026-06', 75, { sample_size: 55 }),
        row('2026-07', 62, { sample_size: 104 }),
        row('2026-08', 45, { sample_size: 548 }),
        row('2026-10', 42, { sample_size: 2000 }),
      ]),
      '2026-12',
    );
    // 104/55 is under 3x; 548/104 is over; October follows a gap, so it has no
    // neighbour to compare with and is not flagged.
    expect(model.points.map((p) => p.mixShift)).toEqual([false, false, true, false]);
  });
});

