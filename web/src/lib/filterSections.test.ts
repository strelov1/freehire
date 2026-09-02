import { describe, expect, it } from 'vitest';
import { FACETS } from './facets';
import { RAIL } from './filterSections';

// Which pane hosts a facet that has no rail entry of its own, and why. The rail is the
// ONLY way into a job facet from the interface: a param with no entry is still parsed
// from the URL and still sent to the API, but nothing in the modal can select it. That
// is exactly how `reality` sat unreachable — declared in FACETS, understood by the
// backend, invisible. The company rail has carried this cover for a while
// (companyRailGroups.test.ts); the job rail did not, which is why nothing caught it.
//
// An entry here is a claim that the facet IS reachable, just not from its own rail row.
// `source` is the one facet that is deliberately not offered at all.
const HOSTED_ELSEWHERE: Record<string, string> = {
  role: 'category pane (the role picker)',
  category: 'category pane (specialization chips)',
  ai_archetype: 'category pane (AI specialization)',
  seniority: 'experience pane',
  role_type: 'experience pane',
  regions: 'location pane (the region → country tree)',
  countries: 'location pane',
  cities: 'location pane',
  work_mode: 'work pane',
  employment_type: 'work pane',
  domains: 'industry pane',
  company_type: 'industry pane',
  collections: 'industry pane, and the employer-credential subset in relocation',
  english_level: 'language pane',
  posting_language: 'language pane',
  relocation: 'relocation pane',
  salary_currency: 'salary pane, beside the minimum',
};

// Not offered anywhere, on purpose. Which job board a posting was crawled from is
// provenance, not something a candidate filters on; the param stays URL-only.
const NOT_OFFERED = new Set(['source']);

describe('the job filter rail', () => {
  it('reaches every job facet — by its own entry, a hosting pane, or a recorded refusal', () => {
    const railParams = new Set(RAIL.map((e) => e.facetParam).filter((p): p is string => !!p));
    const unreachable = FACETS.map((f) => f.param).filter(
      (p) => !railParams.has(p) && !(p in HOSTED_ELSEWHERE) && !NOT_OFFERED.has(p),
    );

    expect(unreachable).toEqual([]);
  });

  it('keeps the exception lists honest — no entry claims a facet that has its own row', () => {
    const railParams = new Set(RAIL.map((e) => e.facetParam).filter((p): p is string => !!p));
    const declared = new Set(FACETS.map((f) => f.param));

    for (const param of [...Object.keys(HOSTED_ELSEWHERE), ...NOT_OFFERED]) {
      expect(declared, `${param} is excepted but is not a declared facet`).toContain(param);
      expect(railParams, `${param} is excepted but also has its own rail entry`).not.toContain(param);
    }
  });

  it('names each pane with a distinct key', () => {
    const keys = RAIL.map((e) => e.key);

    expect(new Set(keys).size).toBe(keys.length);
  });
});

describe('the Experience pane', () => {
  it('is a ROLE-section rail entry of its own', () => {
    const entry = RAIL.find((e) => e.key === 'experience');
    expect(entry).toBeDefined();
    expect(entry?.kind).toBe('experience');
    expect(entry?.section).toBe('ROLE');
  });

  // Seniority is a control of the Experience pane, not a facet entry of its own —
  // FilterModal renders it there. A standalone entry would put the same pills in
  // two places in the rail.
  it('leaves seniority without a standalone rail entry', () => {
    expect(RAIL.find((e) => e.key === 'seniority')).toBeUndefined();
    expect(RAIL.find((e) => e.facetParam === 'seniority')).toBeUndefined();
  });
});

describe('the ai_archetype facet', () => {
  it('has no standalone rail entry — FilterModal folds it into the Role pane instead', () => {
    expect(RAIL.find((e) => e.key === 'ai_archetype')).toBeUndefined();
    expect(RAIL.find((e) => e.facetParam === 'ai_archetype')).toBeUndefined();
  });
});

// How old a posting is describes the POSTING, not something asked of the candidate, so
// `REQUIREMENTS & ELIGIBILITY` — where it sat as the rail's last row — both misfiled it
// and buried it. It belongs beside Experience, the other "how does this posting stand
// relative to me" question.
describe('the Posted pane', () => {
  it('sits in ROLE, adjacent to Experience', () => {
    const posted = RAIL.findIndex((e) => e.key === 'posted');
    const experience = RAIL.findIndex((e) => e.key === 'experience');

    expect(posted).toBeGreaterThan(-1);
    expect(RAIL[posted]?.section).toBe('ROLE');
    expect(posted).toBe(experience + 1);
  });
});

describe('the Posting reality pane', () => {
  it('has a rail entry rendering the reality facet', () => {
    const entry = RAIL.find((e) => e.facetParam === 'reality');

    expect(entry).toBeDefined();
    expect(entry?.kind).toBe('facet');
    expect(entry?.section).toBe('ROLE');
  });
});
