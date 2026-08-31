import { describe, expect, it } from 'vitest';
import { slugifiedCountries } from './facets';
import { CATEGORY_VALUES } from './generated/contracts';
import {
  categoryFromSlug,
  categorySlug,
  englishBreakdown,
  freshCount,
  landingCategories,
  categoryLandingLink,
  landingIntro,
  MIN_PAIR_OPEN,
  MIN_SALARY_SAMPLE,
  neighbourCategories,
  neighbourCountries,
  publishedCountries,
  publishableSalaryBands,
} from './roleLandings';
import type { InsightSalaryBand } from './api';

const band = (over: Partial<InsightSalaryBand> = {}): InsightSalaryBand => ({
  seniority: '',
  currency: 'EUR',
  period: 'year',
  sample_size: 40,
  p25: 70000,
  p50: 90000,
  p75: 110000,
  ...over,
});

describe('the category axis', () => {
  it('renders underscores as hyphens and back', () => {
    expect(categorySlug('data_analytics')).toBe('data-analytics');
    expect(categoryFromSlug('data-analytics')).toBe('data_analytics');
  });

  it('round-trips every generated category', () => {
    for (const { category, slug } of landingCategories()) {
      expect(categoryFromSlug(slug)).toBe(category);
      expect(categorySlug(category)).toBe(slug);
    }
  });

  it('refuses a slug that names no category', () => {
    expect(categoryFromSlug('not-a-category')).toBeUndefined();
    // 'other' is a real vocabulary value but names nothing a searcher looks for.
    expect(categoryFromSlug('other')).toBeUndefined();
  });

  it('publishes every generated category except the catch-all', () => {
    const published = landingCategories().map((c) => c.category).toSorted();
    const expected = CATEGORY_VALUES.filter((c) => c !== 'other').toSorted();
    expect(published).toEqual(expected);
  });
});

describe('the publication gate', () => {
  const counts = { de: 2041, pl: 300, fr: MIN_PAIR_OPEN, it: MIN_PAIR_OPEN - 1, zz: 9999 };

  it('admits a country at the floor and rejects the one below it', () => {
    const published = publishedCountries(counts).map((c) => c.code);
    expect(published).toContain('fr');
    expect(published).not.toContain('it');
  });

  it('drops a country code that carries no slug, however many jobs it holds', () => {
    // 'zz' is not an ISO country; without this it would reach a URL as a raw code.
    expect(publishedCountries(counts).map((c) => c.code)).not.toContain('zz');
  });

  it('orders by demand, so the biggest markets lead the table', () => {
    expect(publishedCountries(counts).map((c) => c.code)).toEqual(['de', 'pl', 'fr']);
  });

  it('carries the slug and display label each row needs to link and render', () => {
    const de = publishedCountries(counts).find((c) => c.code === 'de');
    expect(de).toMatchObject({ slug: 'germany', label: 'Germany', openCount: 2041 });
  });
});

describe('the salary honesty rule', () => {
  it('hides a band whose sample is too small to be a measurement', () => {
    const bands = publishableSalaryBands([
      band({ sample_size: MIN_SALARY_SAMPLE }),
      band({ sample_size: MIN_SALARY_SAMPLE - 1, currency: 'USD' }),
    ]);
    expect(bands.map((b) => b.currency)).toEqual(['EUR']);
  });

  it('never combines currencies — each is its own row', () => {
    // backend x pl really does return PLN year, PLN month, PLN hour and USD month.
    const bands = publishableSalaryBands([
      band({ currency: 'PLN', period: 'year', sample_size: 34 }),
      band({ currency: 'PLN', period: 'month', sample_size: 24 }),
      band({ currency: 'USD', period: 'month', sample_size: 60 }),
    ]);
    expect(bands).toHaveLength(3);
  });

  it('leads with the richest sample, so the headline figure is the best-evidenced one', () => {
    const bands = publishableSalaryBands([
      band({ currency: 'USD', sample_size: 12 }),
      band({ currency: 'EUR', sample_size: 900 }),
    ]);
    expect(bands[0]?.currency).toBe('EUR');
  });

  it('returns nothing when every band is under-sampled', () => {
    expect(publishableSalaryBands([band({ sample_size: 3 })])).toEqual([]);
  });
});

describe('the english-level honesty rule', () => {
  it('stays silent when the annotation covers too little of the pair', () => {
    // Measured on prod: backend x de annotates 136 of 2041 postings (6.7%). A
    // confident sentence off that sample describes the annotation, not the market.
    expect(englishBreakdown({ c1: 123, b2: 6, b1: 5, a2: 1, c2: 1 }, 2041)).toBeNull();
  });

  it('reports once the annotation covers enough, as a share of those that declare', () => {
    const rows = englishBreakdown({ c1: 300, b2: 100 }, 1000);
    expect(rows).not.toBeNull();
    // 400 of 1000 clears the floor; c1 is 300 of the 400 that DECLARE, not of 1000.
    expect(rows?.[0]).toMatchObject({ level: 'c1', count: 300, share: 0.75 });
  });

  it('stays silent on an empty distribution rather than dividing by zero', () => {
    expect(englishBreakdown({}, 1000)).toBeNull();
  });
});

describe('the reality framing', () => {
  it('reads the fresh count, never the stale one', () => {
    // The same pair reads stale:1568 fresh:470. "77% stale" is true and indicts the
    // catalogue rather than the market; the fresh count carries the same signal.
    expect(freshCount({ stale: 1568, fresh: 470, 'likely-evergreen': 3 })).toBe(470);
  });

  it('is zero when nothing is fresh, not undefined', () => {
    expect(freshCount({ stale: 10 })).toBe(0);
  });
});

describe('the auto-intro', () => {
  const intro = () =>
    landingIntro({
      category: 'backend',
      countryCode: 'de',
      total: 2041,
      fresh: 470,
      topSkills: ['python', 'aws', 'kubernetes'],
    });

  it('is deterministic for fixed input', () => {
    expect(intro()).toBe(intro());
  });

  it('states the figures a searcher came for', () => {
    const text = intro();
    expect(text).toContain('2,041');
    expect(text).toContain('Backend');
    expect(text).toContain('Germany');
    expect(text).toContain('470');
    expect(text).toContain('python');
  });

  it('drops the freshness clause when nothing is fresh rather than writing a zero', () => {
    const text = landingIntro({
      category: 'backend',
      countryCode: 'de',
      total: 60,
      fresh: 0,
      topSkills: ['go'],
    });
    expect(text).not.toContain('posted recently');
    expect(text).toContain('60');
  });

  it('survives a pair with no skills annotated', () => {
    const text = landingIntro({
      category: 'legal',
      countryCode: 'pl',
      total: 51,
      fresh: 4,
      topSkills: [],
    });
    expect(text).toContain('51');
    expect(text).not.toContain('undefined');
  });
});

describe('internal linking', () => {
  it('offers the same category in other countries, excluding the current one', () => {
    const rows = neighbourCountries({ de: 2041, pl: 300, fr: 80, it: 10 }, 'de');
    expect(rows.map((r) => r.code)).toEqual(['pl', 'fr']);
  });

  it('offers other categories in this country, excluding the current one', () => {
    const rows = neighbourCategories({ backend: 2041, devops: 300, other: 900 }, 'backend');
    expect(rows.map((r) => r.category)).toEqual(['devops']);
  });

  it('caps each neighbour list so the footer stays a link block, not a directory', () => {
    const many = Object.fromEntries(
      slugifiedCountries().map((c, i) => [c.code, 100 + i])
    );
    expect(Object.keys(many).length).toBeGreaterThan(8);
    expect(neighbourCountries(many, 'de').length).toBeLessThanOrEqual(8);
  });
});

describe('the link a job page offers', () => {
  it('points at the category table, which is published whenever any country is', () => {
    expect(categoryLandingLink('backend')).toMatchObject({
      category: 'backend',
      slug: 'backend',
      label: 'Backend',
    });
  });

  it('offers nothing for the catch-all category', () => {
    expect(categoryLandingLink('other')).toBeNull();
  });

  it('offers nothing when the posting carries no category', () => {
    expect(categoryLandingLink(null)).toBeNull();
    expect(categoryLandingLink(undefined)).toBeNull();
    expect(categoryLandingLink('')).toBeNull();
  });

  it('offers nothing for a value outside the vocabulary', () => {
    expect(categoryLandingLink('astrology')).toBeNull();
  });
});
