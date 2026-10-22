import { describe, expect, it } from 'vitest';
import {
  breakdownLabel,
  dominantCurrency,
  formatGrowth,
  hiringRobots,
  replyRate,
  salaryRange,
  topEntries,
} from './companyHiring';

describe('topEntries', () => {
  it('ranks by count, then by value, and keeps the first n', () => {
    expect(topEntries({ go: 3, rust: 7, java: 3, sql: 1 }, 3)).toEqual([
      { value: 'rust', count: 7 },
      { value: 'go', count: 3 },
      { value: 'java', count: 3 },
    ]);
  });

  it('drops zero counts and survives a missing distribution', () => {
    expect(topEntries({ a: 0 }, 5)).toEqual([]);
    expect(topEntries(undefined, 5)).toEqual([]);
  });
});

describe('dominantCurrency', () => {
  it('is the currency most postings are paid in', () => {
    expect(dominantCurrency({ EUR: 3, USD: 30, INR: 2 })).toBe('USD');
  });

  it('is null when no posting states a currency', () => {
    expect(dominantCurrency({})).toBeNull();
    expect(dominantCurrency(undefined)).toBeNull();
  });
});

describe('salaryRange', () => {
  const stats = { salary_min: { min: 120000, max: 274456 }, salary_max: { min: 180000, max: 334600 } };

  it('spans the lowest floor to the highest ceiling', () => {
    expect(salaryRange(stats, 21)).toEqual({ min: 120000, max: 334600 });
  });

  it('is null on too small a sample, so two postings never read as a pay band', () => {
    expect(salaryRange(stats, 2)).toBeNull();
  });

  it('falls back to the one bound the postings state', () => {
    expect(salaryRange({ salary_min: { min: 90000, max: 110000 } }, 5)).toEqual({ min: 90000, max: 110000 });
  });

  it('is null when no posting states a salary', () => {
    expect(salaryRange({}, 10)).toBeNull();
  });
});

describe('formatGrowth', () => {
  it('signs the change', () => {
    expect(formatGrowth(12)).toBe('+12');
    expect(formatGrowth(-5)).toBe('−5');
    expect(formatGrowth(0)).toBe('no change');
  });
});

describe('breakdownLabel', () => {
  it('names a value the way the Jobs tab filter names it', () => {
    expect(breakdownLabel('work_mode', 'onsite')).toBe('On-site');
    expect(breakdownLabel('seniority', 'c_level')).toBe('C-level');
    expect(breakdownLabel('countries', 'de')).toBe('Germany');
  });

  it('falls back to the dynamic label for a value with no static option', () => {
    expect(breakdownLabel('skills', 'ci-cd')).toBe('CI/CD');
  });
});

describe('hiringRobots', () => {
  const quiet = [{ period: '2026-10-01', added: 0, removed: 0 }];

  it('keeps an empty company out of the index', () => {
    expect(hiringRobots({ open: 0, open_prev: 0, growth: 0, series: quiet })).toBe('noindex, follow');
  });

  it('indexes a company with anything to read', () => {
    expect(hiringRobots({ open: 3, open_prev: 0, growth: 3, series: quiet })).toBeUndefined();
    expect(
      hiringRobots({ open: 0, open_prev: 2, growth: -2, series: [{ period: '2026-10-01', added: 0, removed: 2 }] }),
    ).toBeUndefined();
  });

  it('does not spell a failed read as an empty company', () => {
    expect(hiringRobots(null)).toBeUndefined();
  });
});

describe('replyRate', () => {
  it('is the answered share as a whole percent', () => {
    expect(replyRate({ applications: 30, answered: 12, unanswered: 18 })).toBe(40);
  });
});
