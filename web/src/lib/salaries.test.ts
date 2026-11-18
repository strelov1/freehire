import { describe, it, expect } from 'vitest';
import type { InsightSalaryMonth } from './api';
import {
  buildCompareChart,
  histogramFromQuantiles,
  lineMonths,
  pageUnit,
  pageUnits,
  readSalariesQuery,
  salariesSearch,
  MAX_COMPARE,
} from './salaries';

describe('salaries address', () => {
  it('reads the scope, the comparison lines and the unit', () => {
    const q = readSalariesQuery(
      new URLSearchParams('category=Backend&seniority=senior&country=DE&vs=frontend.de,.pl&unit=EUR-year'),
    );
    expect(q).toEqual({
      scope: { category: 'backend', seniority: 'senior', country: 'de' },
      compare: [
        { category: 'frontend', country: 'de' },
        { category: '', country: 'pl' },
      ],
      unit: 'EUR-year',
    });
  });

  it('caps the comparison lines and round-trips through the href', () => {
    const vs = Array.from({ length: 9 }, (_, i) => `r${i}.`).join(',');
    const q = readSalariesQuery(new URLSearchParams(`category=backend&vs=${vs}`));
    expect(q.compare).toHaveLength(MAX_COMPARE);
    expect(readSalariesQuery(new URLSearchParams(salariesSearch(q)))).toEqual(q);
  });

  it('keys a line by its parsed pair, so trailing text cannot repeat a line', () => {
    const q = readSalariesQuery(new URLSearchParams('vs=frontend.de,frontend.de.extra'));
    expect(q.compare).toEqual([{ category: 'frontend', country: 'de' }]);
  });

  it("drops a repeated line and one equal to the scope's own", () => {
    const q = readSalariesQuery(
      new URLSearchParams('category=backend&country=de&vs=backend.de,frontend.,FRONTEND.,.pl'),
    );
    expect(q.compare).toEqual([
      { category: 'frontend', country: '' },
      { category: '', country: 'pl' },
    ]);
  });

  it('writes an empty query as no query string', () => {
    expect(salariesSearch(readSalariesQuery(new URLSearchParams()))).toBe('');
  });
});

describe('histogramFromQuantiles', () => {
  // Evenly spread from 50k to 140k: 5k per 5% step.
  const even = Array.from({ length: 19 }, (_, i) => 50000 + i * 5000);

  it('covers the middle 90% in round bins whose shares sum to 0.9', () => {
    const bins = histogramFromQuantiles(even);
    expect(bins[0]?.lo).toBe(50000);
    expect(bins.every((b) => (b.hi - b.lo) % 1000 === 0)).toBe(true);
    expect(bins.reduce((s, b) => s + b.share, 0)).toBeCloseTo(0.9, 6);
  });

  it('puts the mass where the quantiles bunch', () => {
    // Half the postings (25th to 75th percentile) sit between 100k and 101k.
    const bunched = [...even];
    for (let i = 4; i <= 14; i++) bunched[i] = 100000 + (i - 4) * 100;
    const bins = histogramFromQuantiles(bunched);
    const peak = bins.reduce((a, b) => (b.share > a.share ? b : a));
    expect(peak.lo).toBeLessThanOrEqual(100000);
    expect(peak.hi).toBeGreaterThan(100000);
    expect(peak.share).toBeGreaterThan(0.5);
  });

  it('has no histogram without a full, non-flat set', () => {
    expect(histogramFromQuantiles([])).toEqual([]);
    expect(histogramFromQuantiles(undefined)).toEqual([]);
    expect(histogramFromQuantiles(Array(19).fill(5000))).toEqual([]);
  });
});

describe('pageUnit', () => {
  const months = (currency: string, period: string, sample_size = 40) =>
    ['2026-07', '2026-08', '2026-09'].map((month) => ({
      currency,
      period,
      month,
      sample_size,
      p25: 1,
      p50: 2,
      p75: 3,
    }));
  const band = {
    seniority: '',
    currency: 'EUR',
    period: 'year',
    sample_size: 9,
    p25: 1,
    p50: 2,
    p75: 3,
    quantiles: [],
  };

  it("defaults to the scope's richest band and honours a request the page can show", () => {
    expect(pageUnit([band], '', [months('USD', 'year')])).toBe('EUR-year');
    expect(pageUnit([band], 'USD-year', [months('USD', 'year')])).toBe('USD-year');
    expect(pageUnit([band], 'GBP-year', [months('USD', 'year')])).toBe('EUR-year');
  });

  it("offers every band's unit, then units only a line's history has", () => {
    const histories = [months('EUR', 'year'), [...months('USD', 'year'), ...months('PLN', 'month', 10)]];
    expect(pageUnits([band], histories)).toEqual(['EUR-year', 'USD-year', 'PLN-month']);
    expect(pageUnits([], [null])).toEqual([]);
  });

  it('lets a comparison line carry the chart when the scope has no figures', () => {
    const histories = [null, [...months('USD', 'year'), ...months('PLN', 'month', 10)]];
    expect(pageUnit([], '', histories)).toBe('USD-year');
    expect(pageUnit([], 'PLN-month', histories)).toBe('PLN-month');
    expect(pageUnit([], 'GBP-year', histories)).toBe('USD-year');
    expect(pageUnit([], '', [null, []])).toBe('');
  });
});

describe('compare chart', () => {
  const m = (month: string, p50: number, currency = 'USD', period = 'year'): InsightSalaryMonth => ({
    currency,
    period,
    month,
    sample_size: 40,
    p25: p50 - 1,
    p50,
    p75: p50 + 1,
  });

  it("keeps one line's months in one unit only, with the single-line chart's floor", () => {
    const rows = [m('2026-07', 1), m('2026-08', 2), m('2026-09', 3), m('2026-09', 9, 'EUR')];
    expect(lineMonths(rows, 'USD-year').map((r) => r.p50)).toEqual([1, 2, 3]);
    expect(lineMonths(rows, 'EUR-year')).toEqual([]); // one month is not a line
    expect(lineMonths(null, 'USD-year')).toEqual([]);
  });

  it('lays every line over one month axis and one scale, breaking at a gap', () => {
    const a = [m('2026-07', 100), m('2026-08', 110), m('2026-10', 120)];
    const b = [m('2026-08', 200), m('2026-09', 210)];
    const model = buildCompareChart([a, b], '2026-10');
    expect(model?.slots).toEqual(['2026-07', '2026-08', '2026-09', '2026-10']);
    expect(model?.min).toBe(100);
    expect(model?.max).toBe(210);
    expect(model?.lines[0]?.segments.map((s) => s.length)).toEqual([2, 1]);
    expect(model?.lines[0]?.points[2]?.partial).toBe(true);
    // The highest median is at the top of the plot, the lowest at the baseline.
    expect(model?.lines[1]?.points[1]?.y).toBe(model?.topY);
    expect(model?.lines[0]?.points[0]?.y).toBe(model?.baselineY);
    expect(buildCompareChart([[], []], '2026-10')).toBeNull();
  });
});
