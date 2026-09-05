import { describe, expect, it } from 'vitest';
import { formatSalary, requirementGroups } from './enrichment';
import type { Requirement } from './enrichment';
import type { Enrichment } from './types';

const req = (text: string, priority: string): Requirement => ({ text, priority });

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

// The whole result in one comparable value, so an assertion never indexes into a
// possibly empty array: one entry per rendered group, in render order.
const shape = (reqs: Requirement[] | undefined) =>
  requirementGroups(reqs).map((g) => ({ priority: g.priority, texts: g.items.map((r) => r.text) }));

describe('requirementGroups', () => {
  it('returns no groups when the job states no requirements', () => {
    expect(requirementGroups(undefined)).toEqual([]);
    expect(requirementGroups([])).toEqual([]);
  });

  it('puts required before preferred regardless of the stored order', () => {
    // Within a group the posting's own order is what the reader saw, so it is kept.
    expect(
      shape([
        req('Kubernetes', 'preferred'),
        req('5+ years Go', 'required'),
        req("Bachelor's degree", 'preferred'),
        req('Postgres', 'required'),
      ]),
    ).toEqual([
      { priority: 'required', texts: ['5+ years Go', 'Postgres'] },
      { priority: 'preferred', texts: ['Kubernetes', "Bachelor's degree"] },
    ]);
  });

  it('omits a group with no entries rather than rendering an empty heading', () => {
    expect(shape([req('5+ years Go', 'required')])).toEqual([
      { priority: 'required', texts: ['5+ years Go'] },
    ]);
  });

  it('carries a display label for each group', () => {
    const groups = requirementGroups([req('Go', 'required'), req('Rust', 'preferred')]);

    expect(groups.map((g) => g.label)).toEqual(['Required', 'Preferred']);
  });

  it('keeps an entry whose text duplicates a skill chip', () => {
    // The list is a quotation from the posting; de-duplicating it against the
    // skills facet would put a second normaliser in the SPA (see the design doc).
    expect(shape([req('Docker', 'required')])).toEqual([
      { priority: 'required', texts: ['Docker'] },
    ]);
  });

  it('treats an entry with an unexpected priority as preferred, like the server does', () => {
    // enrich's coerceRequirementPriority sends anything that is not `required` to
    // `preferred`. The server coerces on write so this should never arrive, but the
    // page must not answer differently from the store if it ever does.
    expect(shape([req('Go', ''), req('Rust', 'nice-to-have')])).toEqual([
      { priority: 'preferred', texts: ['Go', 'Rust'] },
    ]);
  });

  it('matches the required priority case- and whitespace-insensitively', () => {
    expect(shape([req('Go', ' Required ')])).toEqual([{ priority: 'required', texts: ['Go'] }]);
  });

  it('drops an entry whose text is blank', () => {
    expect(shape([req('  ', 'required'), req('Go', 'required')])).toEqual([
      { priority: 'required', texts: ['Go'] },
    ]);
  });
});
