import { describe, expect, it } from 'vitest';
import { formatSalary } from './enrichment';
import type { Enrichment } from './types';

describe('formatSalary', () => {
  it('returns null when neither bound is stated', () => {
    expect(formatSalary({} as Enrichment)).toBeNull();
  });

  it('compacts a full range to K, with the currency trailing', () => {
    expect(
      formatSalary({ salary_min: 30_000, salary_max: 50_000, salary_currency: 'GBP' } as Enrichment),
    ).toBe('30K – 50K £');
  });

  it('rounds a non-round thousand to the nearest K', () => {
    expect(formatSalary({ salary_min: 45_500, salary_max: 45_500 } as Enrichment)).toBe('46K – 46K');
  });

  it('compacts millions with one decimal only when not round', () => {
    expect(formatSalary({ salary_min: 1_200_000, salary_max: 2_000_000 } as Enrichment)).toBe(
      '1.2M – 2M',
    );
  });

  it('compacts billions the same way as millions', () => {
    expect(formatSalary({ salary_min: 1_000_000_000, salary_max: 1_500_000_000 } as Enrichment)).toBe(
      '1B – 1.5B',
    );
  });

  it('leaves sub-thousand amounts (e.g. an hourly rate) uncompacted', () => {
    expect(
      formatSalary({ salary_min: 40, salary_max: 60, salary_currency: 'USD', salary_period: 'hour' } as Enrichment),
    ).toBe('40 – 60 $ / hr');
  });

  it('renders a min-only floor as "from …"', () => {
    expect(formatSalary({ salary_min: 100_000, salary_currency: 'USD' } as Enrichment)).toBe(
      'from 100K $',
    );
  });

  it('renders a max-only ceiling as "up to …"', () => {
    expect(formatSalary({ salary_max: 80_000, salary_currency: 'EUR' } as Enrichment)).toBe(
      'up to 80K €',
    );
  });

  it('falls back to the raw currency code when it is not in the symbol map', () => {
    expect(formatSalary({ salary_min: 10_000, salary_max: 20_000, salary_currency: 'PLN' } as Enrichment)).toBe(
      '10K – 20K PLN',
    );
  });
});
