import { describe, expect, it } from 'vitest';

import { filtersFromProfile, filtersToParams } from './facetModel';
import {
  answerChips,
  matchCompanies,
  matchCompaniesLine,
  companiesLine,
  asProfile,
  currentQuestion,
  matchQuery,
  matchSkills,
  nextPhase,
  profileFromAnswers,
  roleChoices,
  roleFacetQuery,
  whereChoices,
  widenedWindow,
  type Answers,
} from './onboardingFlow';
import type { Job, OnboardingParse } from './types';

function parse(over: Partial<OnboardingParse> = {}): OnboardingParse {
  return {
    structured_available: true,
    first_name: 'Ana',
    headline: 'Backend engineer',
    country: 'br',
    total_years: 9,
    companies: ['Nubank', 'Stone'],
    skills: ['go', 'kubernetes', 'postgresql'],
    categories: ['backend', 'devops'],
    seniority: 'senior',
    report: {
      overall: 70,
      potential: 90,
      categories: [],
      strong_keywords: [],
      recommended_keywords: [],
      reviewed: false,
    },
    ...over,
  };
}

const ids = (choices: { id: string }[]) => choices.map((c) => c.id);

describe('nextPhase', () => {
  it('walks the flow from drop to results', () => {
    expect(nextPhase('drop', 'file')).toBe('parsing');
    expect(nextPhase('parsing', 'parsed')).toBe('questions');
    expect(nextPhase('questions', 'answered')).toBe('searching');
    expect(nextPhase('searching', 'found')).toBe('results');
    expect(nextPhase('results', 'carry')).toBe('carrying');
  });

  it('returns to the drop step when the parse fails, and to the questions when the search does', () => {
    expect(nextPhase('parsing', 'failed')).toBe('drop');
    expect(nextPhase('searching', 'failed')).toBe('questions');
  });

  it('lets results go back to a question, and anything start over', () => {
    expect(nextPhase('results', 'edit')).toBe('questions');
    for (const p of ['parsing', 'questions', 'searching', 'results'] as const) {
      expect(nextPhase(p, 'restart')).toBe('drop');
    }
  });

  it('ignores an event the phase does not take', () => {
    expect(nextPhase('drop', 'found')).toBe('drop');
    expect(nextPhase('results', 'file')).toBe('results');
  });
});

describe('currentQuestion', () => {
  it('asks the first unanswered question in order, then nothing', () => {
    expect(currentQuestion({})).toBe('where');
    expect(currentQuestion({ where: 'remote' })).toBe('role');
    expect(currentQuestion({ where: 'remote', role: 'any' })).toBe('challenge');
    expect(currentQuestion({ where: 'remote', role: 'any', challenge: ['english'] })).toBeNull();
  });

  // Continue with nothing picked is an answer: no preference.
  it('takes an empty challenge list as answered', () => {
    expect(currentQuestion({ where: 'remote', role: 'any', challenge: [] })).toBeNull();
  });
});

describe('whereChoices', () => {
  it('offers the CV country when it resolved', () => {
    const choices = whereChoices(parse());
    expect(ids(choices)).toEqual(['remote', 'country', 'anywhere', 'any']);
    expect(choices[1]?.label).toBe('Brazil');
  });

  it('offers remote, anywhere and no preference only when it did not', () => {
    expect(ids(whereChoices(parse({ country: '' })))).toEqual(['remote', 'anywhere', 'any']);
  });
});

describe('roleChoices', () => {
  it('pairs each CV category with the seniority, plus no preference', () => {
    expect(roleChoices(parse())).toEqual([
      { id: 'backend', label: 'Senior Backend' },
      { id: 'devops', label: 'Senior DevOps' },
      { id: 'any', label: 'No preference' },
    ]);
  });

  it('caps at four categories and drops the level when none was derived', () => {
    const choices = roleChoices(
      parse({ categories: ['backend', 'devops', 'sre', 'frontend', 'qa'], seniority: undefined }),
    );
    expect(ids(choices)).toEqual(['backend', 'devops', 'sre', 'frontend', 'any']);
    expect(choices[0]?.label).toBe('Backend');
  });

  it("puts the CV's own roles first, then fills up with what the catalogue holds for its skills", () => {
    const counts = { devops: 3440, fullstack: 3411, backend: 2019, architecture: 1110, frontend: 1082 };
    expect(roleChoices(parse(), counts)).toEqual([
      { id: 'backend', label: 'Senior Backend', count: 2019 },
      { id: 'devops', label: 'Senior DevOps', count: 3440 },
      { id: 'fullstack', label: 'Senior Full-Stack', count: 3411 },
      { id: 'architecture', label: 'Senior Architecture', count: 1110 },
      { id: 'any', label: 'No preference' },
    ]);
  });

  it('leaves out a role with no open jobs, even one the CV names', () => {
    expect(ids(roleChoices(parse(), { devops: 12, frontend: 5 }))).toEqual(['devops', 'frontend', 'any']);
  });

  it('offers only no preference when the catalogue holds nothing for the skills', () => {
    expect(ids(roleChoices(parse(), {}))).toEqual(['any']);
    expect(ids(roleChoices(parse(), { backend: 0 }))).toEqual(['any']);
  });

  it("falls back to the CV's own roles while the catalogue was not asked", () => {
    expect(ids(roleChoices(parse()))).toEqual(['backend', 'devops', 'any']);
  });
});

describe('roleFacetQuery', () => {
  it("counts by the CV's skills under the where answer, with no role or window", () => {
    const q = roleFacetQuery(parse(), { where: 'remote' });
    expect(q.get('skills')?.split(',')).toEqual(parse().skills);
    expect(q.get('work_mode')).toBe('remote');
    expect(q.get('regions')).toBe('global,latam');
    expect(q.get('countries')).toBe('br');
    expect(q.has('category')).toBe(false);
    expect(q.has('seniority')).toBe(false);
    expect(q.has('posted_within_days')).toBe(false);
  });
});

describe('profileFromAnswers', () => {
  it('takes the role and level from the role answer and the skills from the CV', () => {
    const p = profileFromAnswers(parse(), { where: 'remote', role: 'devops' });
    expect(p.specializations).toEqual(['devops']);
    expect(p.seniorities).toEqual(['senior']);
    expect(p.skills).toEqual(['go', 'kubernetes', 'postgresql']);
  });

  it('keeps every CV role and no level under no preference', () => {
    const p = profileFromAnswers(parse(), { role: 'any' });
    expect(p.specializations).toEqual(['backend', 'devops']);
    expect(p.seniorities).toEqual([]);
  });

  it('saves remote as a remote work mode', () => {
    expect(profileFromAnswers(parse(), { where: 'remote' }).location?.work_modes).toEqual(['remote']);
  });

  // "Remote" means remote work this candidate can take from where they are: open
  // worldwide, open to their region, or open to their country.
  it("bounds remote by the CV's country and its region", () => {
    expect(profileFromAnswers(parse(), { where: 'remote' }).location).toEqual({
      work_modes: ['remote'],
      remote: { regions: ['global', 'latam'], countries: ['br'] },
      base: { country: 'br' },
      relocation: { open: false },
    });
  });

  it('keeps remote unbounded when the CV named no country', () => {
    expect(profileFromAnswers(parse({ country: '' }), { where: 'remote' }).location).toEqual({
      work_modes: ['remote'],
      remote: {},
      base: {},
      relocation: { open: false },
    });
  });

  it('saves no location for anywhere or no preference', () => {
    expect(profileFromAnswers(parse(), { where: 'anywhere' }).location).toBeNull();
    expect(profileFromAnswers(parse(), { where: 'any' }).location).toBeNull();
  });
});

// The saved profile's alert has to find what the flow found: the flow's query is the
// profile's own filters (minus skills, which rank instead of filter) plus the answers a
// profile does not hold.
describe('matchQuery', () => {
  const full: Answers = { where: 'country', role: 'backend', challenge: [] };

  function profileParams(answers: Answers): URLSearchParams {
    return filtersToParams(filtersFromProfile(asProfile(profileFromAnswers(parse(), answers))));
  }

  it('carries every answer as a search filter, with the window', () => {
    const q = new URLSearchParams(matchQuery(parse(), full, 7));
    expect(q.get('countries')).toBe('br');
    expect(q.get('category')).toBe('backend');
    expect(q.get('seniority')).toBe('senior');
    expect(q.get('posted_within_days')).toBe('7');
    expect(q.has('skills')).toBe(false);
  });

  it('reproduces the saved profile’s location, role and level filters', () => {
    for (const where of ['remote', 'country', 'anywhere', 'any'] as const) {
      const answers = { ...full, where };
      const q = new URLSearchParams(matchQuery(parse(), answers, 7));
      const fromProfile = profileParams(answers);
      fromProfile.delete('skills');
      for (const [key, value] of fromProfile) expect(q.get(key), key).toBe(value);
    }
  });

  it('maps remote to remote work open worldwide, to the region or to the country', () => {
    const q = new URLSearchParams(matchQuery(parse(), { ...full, where: 'remote' }, 30));
    expect(q.get('work_mode')).toBe('remote');
    expect(q.get('regions')).toBe('global,latam');
    expect(q.get('countries')).toBe('br');
    expect(q.get('posted_within_days')).toBe('30');
  });

  it('maps remote to plain remote work when the CV named no country', () => {
    const q = new URLSearchParams(matchQuery(parse({ country: '' }), { ...full, where: 'remote' }, 30));
    expect(q.get('work_mode')).toBe('remote');
    expect(q.has('regions')).toBe(false);
    expect(q.has('countries')).toBe(false);
  });

  it('adds nothing for the no-preference answers', () => {
    const q = new URLSearchParams(matchQuery(parse(), { where: 'any', role: 'any', challenge: [] }, 7));
    expect(q.has('work_mode')).toBe(false);
    expect(q.has('countries')).toBe(false);
    expect(q.has('visa_sponsorship')).toBe(false);
    expect(q.has('seniority')).toBe(false);
  });

  it('keeps only sponsoring postings for someone who needs a visa', () => {
    const q = new URLSearchParams(matchQuery(parse(), { ...full, challenge: ['visa_sponsorship'] }, 7));
    expect(q.get('visa_sponsorship')).toBe('true');
  });

  it('turns a wish for remote work into remote postings, unless a place was named', () => {
    const want = (where: Answers['where']) =>
      new URLSearchParams(matchQuery(parse(), { ...full, where, challenge: ['remote_flexible'] }, 7));
    expect(want('anywhere').get('work_mode')).toBe('remote');
    expect(want('any').get('work_mode')).toBe('remote');
    expect(want('country').has('work_mode')).toBe(false);
    expect(want('country').get('countries')).toBe('br');
    expect(want('remote').get('work_mode')).toBe('remote');
  });

  it('lets the other challenges leave the search alone', () => {
    const q = new URLSearchParams(matchQuery(parse(), { ...full, challenge: ['english', 'job_fit'] }, 7));
    expect(q.toString()).toBe(matchQuery(parse(), full, 7));
  });
});

describe('matchSkills', () => {
  it('sends at most fifty of the CV skills', () => {
    const many = Array.from({ length: 60 }, (_, i) => `s${i}`);
    expect(matchSkills(parse({ skills: many }))).toHaveLength(50);
  });
});

describe('widenedWindow', () => {
  it('widens an empty week to thirty days, once', () => {
    expect(widenedWindow(7, 0)).toBe(30);
    expect(widenedWindow(7, 3)).toBeNull();
    expect(widenedWindow(30, 0)).toBeNull();
  });
});

describe('companiesLine', () => {
  it('names two employers and counts the rest', () => {
    expect(companiesLine(['Finbridge'])).toBe('Finbridge');
    expect(companiesLine(['Finbridge', 'Sber'])).toBe('Finbridge and Sber');
    expect(companiesLine(['Finbridge', 'Sber', 'Informa'])).toBe('Finbridge, Sber and 1 more');
    expect(companiesLine(['Finbridge', 'Sber', 'Informa', 'RingCentral'])).toBe('Finbridge, Sber and 2 more');
  });

  it('can count what is left in some other unit', () => {
    expect(companiesLine(['Magnit Tech', 'Placements24', 'Acme'], 523, 'jobs')).toBe(
      'Magnit Tech, Placements24 and 523 more jobs',
    );
    expect(companiesLine(['Magnit Tech', 'Placements24'], 0, 'jobs')).toBe('Magnit Tech and Placements24');
  });
});

describe('matchCompanies', () => {
  const m = (company: string) => ({ job: { company } as Job, match_percent: 50 });

  it('lists each employer once, in rank order, at most ten', () => {
    const ms = ['B', 'A', 'B', ...Array.from({ length: 12 }, (_, i) => `C${i}`)].map(m);
    const names = matchCompanies(ms);
    expect(names.slice(0, 3)).toEqual(['B', 'A', 'C0']);
    expect(names).toHaveLength(10);
  });
});

describe('matchCompaniesLine', () => {
  const m = (company: string) => ({ job: { company } as Job, match_percent: 50 });

  it('names two employers and counts the jobs beyond theirs from the total', () => {
    expect(matchCompaniesLine([m('Magnit Tech'), m('Placements24'), m('Magnit Tech'), m('Acme')], 526)).toBe(
      'Magnit Tech, Placements24 and 523 more jobs',
    );
  });

  it('names one or two employers alone when they hold every job', () => {
    expect(matchCompaniesLine([m('Acme'), m('Acme')], 2)).toBe('Acme');
  });
});

describe('answerChips', () => {
  it('names each answer that narrows, in question order', () => {
    expect(
      answerChips(parse(), { where: 'country', role: 'backend', challenge: ['visa_sponsorship', 'english'] }),
    ).toEqual(['Brazil', 'Senior Backend', 'Getting visa sponsorship +1']);
  });

  it('leaves out no-preference answers', () => {
    expect(answerChips(parse(), { where: 'any', role: 'any', challenge: [] })).toEqual([]);
  });
});
