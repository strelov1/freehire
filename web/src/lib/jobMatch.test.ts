import { describe, it, expect } from 'vitest';
import { resolveMatchState, matchBarSegments, computeClientMatch, partitionBlockers } from './jobMatch';

describe('resolveMatchState', () => {
  const base = { jobSkills: ['go'], authenticated: true, profileLoaded: true, profileSkills: ['go'] };

  it('is "no-skills" when the job has no recognised skills (regardless of auth)', () => {
    expect(resolveMatchState({ ...base, jobSkills: [] })).toBe('no-skills');
    expect(resolveMatchState({ ...base, jobSkills: [], authenticated: false })).toBe('no-skills');
  });

  it('is "guest" when not authenticated and the job has skills', () => {
    expect(resolveMatchState({ ...base, authenticated: false })).toBe('guest');
  });

  it('is "loading" when authenticated but the profile has not loaded yet', () => {
    expect(resolveMatchState({ ...base, profileLoaded: false, profileSkills: null })).toBe('loading');
  });

  it('is "no-profile" when authenticated, loaded, but no profile skills', () => {
    expect(resolveMatchState({ ...base, profileSkills: null })).toBe('no-profile');
    expect(resolveMatchState({ ...base, profileSkills: [] })).toBe('no-profile');
  });

  it('is "ready" when authenticated with a non-empty profile and a skilled job', () => {
    expect(resolveMatchState(base)).toBe('ready');
  });
});

describe('matchBarSegments', () => {
  it('splits the bar into a full-weight exact segment and a half-weight adjacent segment', () => {
    // 2 exact + 1 adjacent of 5 → exact 40%, adjacent 10% (0.5*1/5).
    expect(matchBarSegments({ total: 5, exact_count: 2, adjacent_count: 1 })).toEqual({
      exact: 40,
      adjacent: 10,
    });
  });

  it('returns zeros when total is 0', () => {
    expect(matchBarSegments({ total: 0, exact_count: 0, adjacent_count: 0 })).toEqual({
      exact: 0,
      adjacent: 0,
    });
  });
});

describe('computeClientMatch', () => {
  it('counts the exact (case-insensitive) overlap of job skills the user has', () => {
    // 2 of 4 job skills are in the profile → 50%.
    expect(computeClientMatch(['go', 'kafka', 'aws', 'spark'], ['go', 'aws', 'react'])).toEqual({
      total: 4,
      matched: 2,
      percent: 50,
    });
  });

  it('matches regardless of case so canonical slugs never miss on casing', () => {
    expect(computeClientMatch(['Go', 'Kafka'], ['go'])).toEqual({ total: 2, matched: 1, percent: 50 });
  });

  it('rounds the percent to the nearest whole', () => {
    // 1 of 3 → 33.33 → 33.
    expect(computeClientMatch(['go', 'kafka', 'aws'], ['go']).percent).toBe(33);
  });

  it('is a zero match, not a divide-by-zero, when the job has no skills', () => {
    expect(computeClientMatch([], ['go'])).toEqual({ total: 0, matched: 0, percent: 0 });
  });

  it('is a zero match when the user has no skills', () => {
    expect(computeClientMatch(['go', 'kafka'], [])).toEqual({ total: 2, matched: 0, percent: 0 });
  });

  it('does not let duplicate profile skills inflate the count', () => {
    expect(computeClientMatch(['go', 'kafka'], ['go', 'go'])).toEqual({ total: 2, matched: 1, percent: 50 });
  });
});

describe('partitionBlockers', () => {
  const b = (category: string, severity: string, score_cap: number, met: boolean) => ({
    category,
    severity,
    score_cap,
    reason: `${category} reason`,
    action: '',
    met,
  });

  it('splits unmet from met and orders unmet hardest-first', () => {
    const { unmet, met } = partitionBlockers([
      b('location_work_mode', 'soft', 75, false),
      b('certification', 'hard', 60, false),
      b('education', 'medium', 65, true),
    ]);
    expect(unmet.map((x) => x.category)).toEqual(['certification', 'location_work_mode']);
    expect(met.map((x) => x.category)).toEqual(['education']);
  });

  it('handles null/undefined as empty', () => {
    expect(partitionBlockers(null)).toEqual({ unmet: [], met: [] });
    expect(partitionBlockers(undefined)).toEqual({ unmet: [], met: [] });
  });
});
