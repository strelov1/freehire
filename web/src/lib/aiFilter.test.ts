import { describe, expect, it } from 'vitest';
import {
  cvContext,
  filtersFromInterpretation,
  limitToRequestedCountries,
  planParams,
  planPreview,
} from './aiFilter';
import type { Interpretation } from './types';
import { filtersToParams } from './facetModel';

function interpretation(over: Partial<Interpretation> = {}): Interpretation {
  return { summary: '', facets: {}, exclude: {}, query: '', unresolved: [], empty: false, ...over };
}

describe('filtersFromInterpretation', () => {
  it('writes the resolved values and nothing else', () => {
    const f = filtersFromInterpretation(
      interpretation({ facets: { seniority: ['senior'], skills: ['go', 'kubernetes'] } }),
    );
    expect(f.facets.seniority?.include).toEqual(['senior']);
    expect(f.facets.skills?.include).toEqual(['go', 'kubernetes']);
    expect(f.q).toBe('');
    expect(f.visa).toBe(false);
    expect(f.salaryMin).toBeNull();
  });

  it('carries exclusions as excluded values, not as wanted ones', () => {
    const f = filtersFromInterpretation(interpretation({ exclude: { skills: ['php'] } }));
    expect(f.facets.skills?.exclude).toEqual(['php']);
    expect(f.facets.skills?.include ?? []).toEqual([]);
  });

  // The interpretation resolves both sides independently, so a value can arrive on both.
  // Mirror the rule the profile seed and the URL parser already share, rather than
  // inventing a third answer: the two must never self-cancel into an empty filter.
  it('keeps a value wanted when it is somehow both wanted and avoided', () => {
    const f = filtersFromInterpretation(
      interpretation({ facets: { skills: ['go'] }, exclude: { skills: ['go'] } }),
    );
    expect(f.facets.skills?.include).toEqual(['go']);
    expect(f.facets.skills?.exclude ?? []).toEqual([]);
  });

  it('carries the scalars and the free text', () => {
    const f = filtersFromInterpretation(
      interpretation({
        query: 'climate modelling',
        salary_min: 120000,
        posted_within_days: 5,
        experience_years_max: 0,
        visa_sponsorship: true,
      }),
    );
    expect(f.q).toBe('climate modelling');
    expect(f.salaryMin).toBe(120000);
    expect(f.postedWithinDays).toBe(5);
    // Zero is the entry-level filter, not an unset bound.
    expect(f.experienceYearsMax).toBe(0);
    expect(f.visa).toBe(true);
  });

  // The result is applied through the same query-string path a saved search uses, so what
  // it produces has to survive the round trip the URL makes of it.
  it('survives serialization to the search params', () => {
    const params = filtersToParams(
      filtersFromInterpretation(
        interpretation({
          facets: { skills: ['go'], countries: ['pt'] },
          exclude: { countries: ['us'] },
          posted_within_days: 7,
        }),
      ),
    );
    expect(params.get('skills')).toBe('go');
    expect(params.get('countries')).toBe('pt');
    expect(params.get('countries_exclude')).toBe('us');
    expect(params.get('posted_within_days')).toBe('7');
  });

  // A facet the interpretation names but this build's filter has no control for must not
  // reach the URL: it would narrow the results with no chip to show for it and no way to
  // lift it. The two vocabularies drift apart across a deploy, so this is a live case.
  it('drops a facet this build has no filter for', () => {
    const params = filtersToParams(
      filtersFromInterpretation(interpretation({ facets: { not_a_facet: ['x'], skills: ['go'] } })),
    );
    expect(params.get('not_a_facet')).toBeNull();
    expect(params.get('skills')).toBe('go');
  });
});

describe('cvContext', () => {
  it('names seniority, fields and at most 15 skills', () => {
    const skills = Array.from({ length: 20 }, (_, i) => `s${i}`);
    const line = cvContext({ seniority: 'senior', categories: ['backend', 'devops'], skills });
    expect(line).toBe(
      `\n\nFrom my CV: seniority senior; field backend, devops; skills ${skills.slice(0, 15).join(', ')}.`,
    );
  });

  it('is empty when the CV yielded nothing', () => {
    expect(cvContext({ categories: [], skills: [] })).toBe('');
  });
});

describe('planParams', () => {
  it('is the query string Apply writes', () => {
    const p = planParams(interpretation({ facets: { category: ['design'], countries: ['br'] } }));
    expect(p.toString()).toBe('category=design&countries=br');
  });
});

describe('limitToRequestedCountries', () => {
  const counts = { br: 799, mx: 54, us: 50 };

  it('keeps only the countries the plan names, largest first', () => {
    expect(limitToRequestedCountries(counts, interpretation({ facets: { countries: ['br'] } }))).toEqual([
      ['br', 799],
    ]);
  });

  it('keeps every country, largest first, when the plan names none', () => {
    expect(limitToRequestedCountries(counts, interpretation())).toEqual([
      ['br', 799],
      ['mx', 54],
      ['us', 50],
    ]);
  });

  it('drops the countries the plan excludes', () => {
    expect(limitToRequestedCountries(counts, interpretation({ exclude: { countries: ['us'] } }))).toEqual([
      ['br', 799],
      ['mx', 54],
    ]);
  });
});

describe('planPreview', () => {
  it('groups included values under their filter label', () => {
    const groups = planPreview(interpretation({ facets: { countries: ['br'] } }));
    const countries = groups.find((g) => g.chips.some((c) => c.key === 'countries:br'));
    expect(countries?.chips).toEqual([{ key: 'countries:br', text: 'Brazil', exclude: false }]);
  });

  it('draws an excluded value struck, and an included one wins over its exclusion', () => {
    const groups = planPreview(
      interpretation({ facets: { skills: ['go'] }, exclude: { skills: ['go', 'php'] } }),
    );
    const chips = groups.flatMap((g) => g.chips).filter((c) => c.key.startsWith('skills:'));
    expect(chips.map((c) => [c.key, c.exclude])).toEqual([
      ['skills:go', false],
      ['skills:php', true],
    ]);
  });

  it('adds the scalar bounds with the shared labels', () => {
    const groups = planPreview(
      interpretation({ query: 'designer', salary_min: 90000, visa_sponsorship: true }),
    );
    expect(groups.map((g) => [g.label, g.chips[0]?.text])).toEqual([
      ['Search', 'designer'],
      ['Salary', '90,000+'],
      ['Visa', 'Sponsorship'],
    ]);
  });
});
