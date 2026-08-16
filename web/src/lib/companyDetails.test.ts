import { describe, expect, test } from 'vitest';
import type { Company, CompanyInfo } from './types';
import {
  companyBadges,
  companyDescription,
  companyFacts,
  companyLocations,
  companySocials,
  hasCompanyDetails,
} from './companyDetails';

// A company carries a good deal of scoring/voting bookkeeping none of these
// derivations look at, so the fixture fills it once and each test overrides only
// the fields under examination.
function company(over: Partial<Company> = {}, info?: CompanyInfo): Company {
  return {
    slug: 'acme',
    name: 'Acme',
    collections: [],
    created_at: null,
    updated_at: null,
    upvote_count: 0,
    downvote_count: 0,
    my_vote: 0,
    feedback_count: 0,
    feedback_rating_avg: null,
    ...over,
    ...(info ? { company_info: info } : {}),
  };
}

describe('companyDescription', () => {
  test('is the trimmed company-info description', () => {
    expect(companyDescription(company({}, { description: '  Acme builds tools.  ' }))).toBe(
      'Acme builds tools.'
    );
  });

  test('is empty when the company has no company-info at all', () => {
    expect(companyDescription(company())).toBe('');
  });

  test('is empty when the description is only whitespace', () => {
    expect(companyDescription(company({}, { description: '   \n  ' }))).toBe('');
  });
});

describe('companyBadges', () => {
  test('names the three curated YC flags in a fixed order', () => {
    expect(
      companyBadges(company({}, { top_company: true, is_hiring: true, stage: 'Seed' }))
    ).toEqual(['YC Top Company', 'Hiring', 'Seed-stage']);
  });

  test('omits the flags that are absent or false', () => {
    expect(companyBadges(company({}, { top_company: false, is_hiring: true }))).toEqual(['Hiring']);
  });

  test('is empty for an uncurated company', () => {
    expect(companyBadges(company())).toEqual([]);
  });
});

describe('companyFacts', () => {
  test('is empty for a company with nothing recorded', () => {
    expect(companyFacts(company())).toEqual([]);
  });

  test('orders the scalar columns founded, employees, headquarters, type', () => {
    const facts = companyFacts(
      company({
        year_founded: 2015,
        employee_count: 1200,
        hq_country: 'US',
        organization_type: 'Private',
      })
    );
    expect(facts).toEqual([
      { term: 'Founded', value: '2015' },
      { term: 'Employees', value: '1,200' },
      { term: 'Headquarters', value: 'United States', flag: 'US' },
      { term: 'Type', value: 'Private' },
    ]);
  });

  test('places the CEO between headquarters and organisation type', () => {
    expect(
      companyFacts(company({ hq_country: 'US', organization_type: 'Private' }, { ceo: 'Ada Lovelace' }))
    ).toEqual([
      { term: 'Headquarters', value: 'United States', flag: 'US' },
      { term: 'CEO', value: 'Ada Lovelace' },
      { term: 'Type', value: 'Private' },
    ]);
  });

  test('composes a stock listing from exchange and symbol', () => {
    expect(companyFacts(company({}, { stock: { exchange: 'NASDAQ', symbol: 'ACME' } }))).toEqual([
      { term: 'Listed', value: 'NASDAQ: ACME' },
    ]);
  });

  test('falls back to the bare symbol when the exchange is unknown', () => {
    expect(companyFacts(company({}, { stock: { symbol: 'ACME' } }))).toEqual([
      { term: 'Listed', value: 'ACME' },
    ]);
  });

  test('drops a stock entry that carries no symbol', () => {
    expect(companyFacts(company({}, { stock: { exchange: 'NASDAQ' } }))).toEqual([]);
  });

  test('composes a funding line from the present parts only', () => {
    expect(
      companyFacts(company({}, { funding: { type: 'Series C', amount: 250_000_000, year: 2021 } }))
    ).toEqual([{ term: 'Funding', value: 'Series C · $250M · 2021' }]);
    expect(companyFacts(company({}, { funding: { type: 'Seed' } }))).toEqual([
      { term: 'Funding', value: 'Seed' },
    ]);
  });

  test('scales a funding amount to K, M and B', () => {
    const amountOf = (amount: number) =>
      companyFacts(company({}, { funding: { amount } }))[0]?.value;
    expect(amountOf(500_000)).toBe('$500K');
    expect(amountOf(250_000_000)).toBe('$250M');
    expect(amountOf(1_200_000_000)).toBe('$1.2B');
    expect(amountOf(2_000_000_000)).toBe('$2B');
    expect(amountOf(750)).toBe('$750');
  });

  test('lists the parent company and joins the subsidiaries', () => {
    expect(
      companyFacts(company({}, { parent: 'Globex', subsidiaries: ['Initech', 'Umbrella'] }))
    ).toEqual([
      { term: 'Parent', value: 'Globex' },
      { term: 'Subsidiaries', value: 'Initech, Umbrella' },
    ]);
  });

  test('ignores an empty subsidiaries list', () => {
    expect(companyFacts(company({}, { subsidiaries: [] }))).toEqual([]);
  });
});

describe('companySocials', () => {
  test('orders the links site, LinkedIn, X, Facebook, Instagram', () => {
    const socials = companySocials(
      company(
        {},
        {
          instagram: 'https://instagram.com/acme',
          website: 'https://acme.com',
          facebook: 'https://facebook.com/acme',
          linkedin: 'https://linkedin.com/company/acme',
          twitter: 'https://twitter.com/acme',
        }
      )
    );
    expect(socials.map((s) => s.key)).toEqual([
      'website',
      'linkedin',
      'twitter',
      'facebook',
      'instagram',
    ]);
  });

  test('names each link for a screen reader', () => {
    expect(companySocials(company({}, { website: 'https://acme.com' }))).toEqual([
      { key: 'website', label: 'Acme website', href: 'https://acme.com' },
    ]);
  });

  test('is empty for a company with no links', () => {
    expect(companySocials(company())).toEqual([]);
  });

  test('skips blank and whitespace-only entries', () => {
    expect(companySocials(company({}, { website: '   ', linkedin: '' }))).toEqual([]);
  });

  test('trims surrounding whitespace off a link', () => {
    expect(companySocials(company({}, { website: '  https://acme.com  ' }))[0]?.href).toBe(
      'https://acme.com'
    );
  });

  // company_info comes from an external importer, and these values land in an href.
  // A javascript: or data: URL there is script execution on our origin, so the scheme
  // is allow-listed rather than sanitised.
  test('drops any link that is not http or https', () => {
    expect(
      companySocials(
        company(
          {},
          {
            website: 'javascript:alert(1)',
            linkedin: 'data:text/html;base64,PHNjcmlwdD4=',
            twitter: 'file:///etc/passwd',
            facebook: '//evil.example.com',
            instagram: 'https://instagram.com/acme',
          }
        )
      ).map((s) => s.key)
    ).toEqual(['instagram']);
  });

  test('accepts plain http as well as https', () => {
    expect(companySocials(company({}, { website: 'http://acme.com' }))).toHaveLength(1);
  });

  test('drops a value that is not a URL at all', () => {
    expect(companySocials(company({}, { twitter: '@acme' }))).toEqual([]);
  });
});

describe('companyLocations', () => {
  test('is the ISO codes of the offices, upper-cased', () => {
    expect(
      companyLocations(
        company({}, { locations: [{ code: 'us', name: 'United States' }, { code: 'JP' }] })
      )
    ).toEqual(['US', 'JP']);
  });

  test('drops duplicates while keeping the first position', () => {
    expect(
      companyLocations(company({}, { locations: [{ code: 'US' }, { code: 'DE' }, { code: 'us' }] }))
    ).toEqual(['US', 'DE']);
  });

  test('ignores entries with no code', () => {
    expect(companyLocations(company({}, { locations: [{ name: 'Nowhere' }, { code: 'FR' }] }))).toEqual([
      'FR',
    ]);
  });

  test('ignores anything that is not a two-letter code', () => {
    expect(
      companyLocations(company({}, { locations: [{ code: 'USA' }, { code: 'X' }, { code: 'GB' }] }))
    ).toEqual(['GB']);
  });

  test('is empty when the company lists no offices', () => {
    expect(companyLocations(company())).toEqual([]);
  });
});

describe('hasCompanyDetails', () => {
  test('is false when the company has no facts, no badges and no description', () => {
    expect(hasCompanyDetails(company())).toBe(false);
  });

  test('is true on a scalar fact alone', () => {
    expect(hasCompanyDetails(company({ year_founded: 2015 }))).toBe(true);
  });

  test('is true on a curated badge alone', () => {
    expect(hasCompanyDetails(company({}, { is_hiring: true }))).toBe(true);
  });

  test('is true on a description alone', () => {
    expect(hasCompanyDetails(company({}, { description: 'Acme builds tools.' }))).toBe(true);
  });

  test('is false when the only description is whitespace', () => {
    expect(hasCompanyDetails(company({}, { description: '  ' }))).toBe(false);
  });

  test('is true on an outbound link alone', () => {
    expect(hasCompanyDetails(company({}, { website: 'https://acme.com' }))).toBe(true);
  });

  test('is true on an office location alone', () => {
    expect(hasCompanyDetails(company({}, { locations: [{ code: 'US' }] }))).toBe(true);
  });

  test('is false when the only link has a scheme we refuse', () => {
    expect(hasCompanyDetails(company({}, { website: 'javascript:alert(1)' }))).toBe(false);
  });
});
