import { describe, it, expect } from 'vitest';
import {
  resolveMatchState,
  matchBarSegments,
  computeClientMatch,
  matchTeaser,
  teaserChips,
  partitionBlockers,
  claimSkill,
} from './jobMatch';
import { must } from './utils';

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

describe('matchTeaser', () => {
  const SKILLS = ['go', 'kafka', 'aws', 'postgres', 'docker'];
  // Enough distinct slugs to exercise the hash rather than one lucky seed.
  const MANY = Array.from(
    { length: 200 },
    (_, i) => must(matchTeaser(`senior-go-engineer-at-acme-${i}`, SKILLS)),
  );
  // The percent ceiling has three regimes — a fractional bound below five skills, an
  // exact-integer one at five (where 90% would round 4.5 up to a full house), and the
  // flat 90 cap above six. Sampling only one skill count would leave two untested.
  const ACROSS_COUNTS = [2, 3, 4, 5, 6, 10, 36, 100].flatMap((total) => {
    const skills = Array.from({ length: total }, (_, i) => `skill-${i}`);
    return Array.from({ length: 40 }, (_, i) => must(matchTeaser(`job-${total}-${i}`, skills)));
  });

  it('is nothing at all for a job with no skills, leaving the no-skills state to render', () => {
    expect(matchTeaser('some-slug', [])).toBeNull();
  });

  it('is nothing for a one-skill job, which has no have/missing story to tell', () => {
    // "1 of 1 skills" beside an 87% bar is the one figure a viewer can catch out, and a
    // lone chip carries no contrast either way — so such a job shows no teaser at all.
    expect(matchTeaser('one-skill-job', ['go'])).toBeNull();
  });

  it('gives one slug the same figures every time it is derived', () => {
    // The teaser is rendered during SSR and again after hydration, on the card and in
    // the sidebar. Two derivations that disagree put two scores for one job on screen.
    const first = matchTeaser('senior-go-engineer-at-acme', SKILLS);
    const second = matchTeaser('senior-go-engineer-at-acme', SKILLS);
    expect(second).toEqual(first);
    expect([...must(second).missing]).toEqual([...must(first).missing]);
  });

  it('keeps the percent inside the 60-90 teaser band for every slug', () => {
    for (const t of MANY) {
      expect(t.percent).toBeGreaterThanOrEqual(60);
      expect(t.percent).toBeLessThanOrEqual(90);
    }
  });

  it('takes the total from the real skill count of the job', () => {
    expect(must(matchTeaser('a-slug', SKILLS)).total).toBe(5);
    expect(must(matchTeaser('a-slug', ['go', 'kafka'])).total).toBe(2);
  });

  it('derives matched from the percent, so the label cannot contradict the bar', () => {
    for (const t of MANY) {
      expect(t.matched).toBe(Math.round((t.percent / 100) * t.total));
    }
  });

  it('marks exactly the skills the matched count leaves over as missing', () => {
    for (const t of MANY) {
      expect(t.missing.size).toBe(t.total - t.matched);
    }
  });

  it('only ever marks skills the job actually carries', () => {
    for (const t of MANY) {
      for (const name of t.missing) expect(SKILLS).toContain(name);
    }
  });

  it('always shows both tones — every teased job has a held and a missing skill', () => {
    // An all-green row under a 73% bar, or an all-red one, reads as a rendering bug.
    for (const t of MANY) {
      expect(t.missing.size).toBeGreaterThanOrEqual(1);
      expect(t.missing.size).toBeLessThanOrEqual(SKILLS.length - 1);
    }
    expect(must(matchTeaser('two-skill-job', ['go', 'kafka'])).missing.size).toBe(1);
  });

  it('holds the percent band and both tones at every skill count', () => {
    // The ceiling arithmetic is the subtlest line in the teaser; a regression in it (a
    // dropped `- 1`, a changed cap) would still pass a suite that only samples five
    // skills, because five is the one count where the bound happens to be exact.
    for (const t of ACROSS_COUNTS) {
      expect(t.percent).toBeGreaterThanOrEqual(60);
      expect(t.percent).toBeLessThanOrEqual(90);
      expect(t.matched).toBe(Math.round((t.percent / 100) * t.total));
      expect(t.matched).toBeGreaterThanOrEqual(1);
      expect(t.matched).toBeLessThan(t.total);
      expect(t.missing.size).toBe(t.total - t.matched);
    }
  });

  it('can mark any position missing, not just the tail of the row', () => {
    // Which skills read as missing is the one thing the seed actually randomises; a
    // shuffle that degenerated to "always the last ones" would still satisfy every
    // other test here, so pin it to every position being reachable.
    const positions = new Set(MANY.flatMap((t) => [...t.missing].map((s) => SKILLS.indexOf(s))));
    expect(positions.size).toBe(SKILLS.length);
  });

  it('reaches every percent in the band rather than parking on a few', () => {
    // Deterministic, so this is an exact count: 60..89 for a five-skill job (the ceiling
    // keeps 90 out, since it would round to a full house).
    expect(new Set(MANY.map((t) => t.percent)).size).toBe(30);
  });
});

describe('teaserChips', () => {
  const SKILLS = ['go', 'kafka', 'aws', 'postgres', 'docker', 'terraform'];

  it('shows the job’s leading skills untouched when one of them already reads missing', () => {
    expect(teaserChips(SKILLS, new Set(['kafka']), 3)).toEqual(['go', 'kafka', 'aws']);
  });

  it('trades the last chip for a missing skill when the whole window reads held', () => {
    // A three-chip row cut from a six-skill job can miss the only red one entirely; the
    // teaser is meant to show both tones, so the row borrows one from further down.
    expect(teaserChips(SKILLS, new Set(['terraform']), 3)).toEqual(['go', 'kafka', 'terraform']);
  });

  it('borrows the first missing skill, keeping the job’s own order', () => {
    expect(teaserChips(SKILLS, new Set(['docker', 'terraform']), 3)).toEqual([
      'go',
      'kafka',
      'docker',
    ]);
  });

  it('does not borrow into a one-chip row, which would leave it all-missing', () => {
    // Trading the only chip for a missing skill inverts the contrast instead of showing
    // it — a lone red chip is no better than a lone green one.
    expect(teaserChips(SKILLS, new Set(['terraform']), 1)).toEqual(['go']);
  });

  it('shows every skill of a job shorter than the row', () => {
    expect(teaserChips(['go', 'kafka'], new Set(['kafka']), 3)).toEqual(['go', 'kafka']);
  });

  it('leaves an all-held row alone when the job has no missing skill to borrow', () => {
    expect(teaserChips(['go'], new Set(), 3)).toEqual(['go']);
  });

  it('has nothing to show for a job with no skills', () => {
    expect(teaserChips([], new Set(), 3)).toEqual([]);
  });
});

describe('claimSkill', () => {
  // A job of four skills the viewer holds one of exactly, one through a neighbour.
  const match = {
    total: 4,
    exact_count: 1,
    adjacent_count: 1,
    coverage_percent: 38,
    matched: ['docker'],
    adjacent: [{ name: 'azure', via: 'aws' }],
    missing: ['bash', 'powershell'],
  };

  it('moves a missing skill into the held group and recomputes the coverage', () => {
    const after = claimSkill(match, 'bash');
    expect(after.matched).toEqual(['docker', 'bash']);
    expect(after.missing).toEqual(['powershell']);
    expect(after.exact_count).toBe(2);
    // round((2 + 0.5 × 1) / 4 × 100)
    expect(after.coverage_percent).toBe(63);
  });

  it('stops half-weighting a claimed adjacent skill', () => {
    const after = claimSkill(match, 'azure');
    expect(after.matched).toEqual(['docker', 'azure']);
    expect(after.adjacent).toEqual([]);
    expect(after.adjacent_count).toBe(0);
    expect(after.exact_count).toBe(2);
    expect(after.coverage_percent).toBe(50);
  });

  it('leaves a match this job never carried alone', () => {
    expect(claimSkill(match, 'kafka')).toEqual(match);
  });

  it('leaves an already-held skill alone', () => {
    expect(claimSkill(match, 'docker')).toEqual(match);
  });

  it('does not mutate the match it was given', () => {
    claimSkill(match, 'bash');
    expect(match.matched).toEqual(['docker']);
    expect(match.missing).toEqual(['bash', 'powershell']);
    expect(match.coverage_percent).toBe(38);
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
