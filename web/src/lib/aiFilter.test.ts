import { describe, expect, it } from 'vitest';
import { filtersFromInterpretation } from './aiFilter';
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
