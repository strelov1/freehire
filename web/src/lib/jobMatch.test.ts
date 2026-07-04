import { describe, it, expect } from 'vitest';
import { resolveMatchState, matchBarSegments } from './jobMatch';

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
