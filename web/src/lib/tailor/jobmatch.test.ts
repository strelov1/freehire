import { describe, it, expect } from 'vitest';
import { viewJobMatch, impactOf, scoreTone } from './jobmatch';
import type { CvJobMatch } from '$lib/cv';

function resp(over: Partial<NonNullable<CvJobMatch['score']>> = {}): CvJobMatch {
  return {
    available: true,
    score: {
      overall: 72,
      contributing: ['requirements_coverage', 'keyword_match', 'job_title_match'],
      categories: [
        {
          id: 'requirements_coverage',
          label: 'Requirements Coverage',
          earned: 30,
          weight: 40,
          available: true,
          items: [{ points: 30, text: 'Your CV covers 3 of 4 requirements', status: 'warn' }],
        },
        {
          id: 'keyword_match',
          label: 'Keyword Match',
          earned: 24,
          weight: 30,
          available: true,
          items: [],
        },
        {
          id: 'job_title_match',
          label: 'Job Title Match',
          earned: 12,
          weight: 20,
          available: true,
          items: [],
        },
        {
          id: 'seniority_fit',
          label: 'Seniority Fit',
          earned: 0,
          weight: 10,
          available: false,
          reason: "this vacancy's title states no seniority",
        },
      ],
      missing_skills: ['terraform'],
      ...over,
    },
  };
}

describe('viewJobMatch', () => {
  it('is null when there is no score to show', () => {
    expect(viewJobMatch(null)).toBeNull();
    expect(viewJobMatch(undefined)).toBeNull();
    expect(viewJobMatch({ available: false, reason: 'CV rendering is not available' })).toBeNull();
  });

  it('carries every category through in the order served', () => {
    const rows = viewJobMatch(resp())?.rows ?? [];
    expect(rows.map((r) => r.id)).toEqual([
      'requirements_coverage',
      'keyword_match',
      'job_title_match',
      'seniority_fit',
    ]);
  });

  it('shows an available row as earned out of its weight', () => {
    const row = viewJobMatch(resp())?.rows[0];
    expect(row?.available).toBe(true);
    expect(row?.text).toBe('30 / 40');
  });

  // The panel must never render an unavailable category as a zero: that is the whole point
  // of the scoring rule, and a row reading "0 / 10" says the opposite of what happened.
  it('shows an unavailable row as its reason, never as a zero', () => {
    const row = viewJobMatch(resp())?.rows[3];
    expect(row?.available).toBe(false);
    expect(row?.text).not.toContain('0');
    expect(row?.reason).toBe("this vacancy's title states no seniority");
  });

  it('names the vacancy skills the CV is missing', () => {
    expect(viewJobMatch(resp())?.missingSkills).toEqual(['terraform']);
  });

  // A score taken over three categories must not read as one taken over four.
  it('says when the score was taken over fewer than all four categories', () => {
    expect(viewJobMatch(resp())?.partial).toBe(true);
    const whole = viewJobMatch(
      resp({ contributing: ['requirements_coverage', 'keyword_match', 'job_title_match', 'seniority_fit'] }),
    );
    expect(whole?.partial).toBe(false);
  });

  it('splits the requirement ledger into checked and unverifiable', () => {
    const v = viewJobMatch(
      resp({
        requirements: [
          { text: 'Proficiency in Python', priority: 'required', coverage: 'covered', skills: ['python'] },
          { text: 'Experience with Terraform', priority: 'required', coverage: 'missing', skills: ['terraform'], missing: ['terraform'] },
          { text: 'Strong communication', priority: 'preferred', coverage: 'unverifiable', cached_status: 'covered' },
        ],
      }),
    );
    expect(v?.requirements.covered.map((r) => r.text)).toEqual(['Proficiency in Python']);
    expect(v?.requirements.missing.map((r) => r.text)).toEqual(['Experience with Terraform']);
    expect(v?.requirements.unverifiable.map((r) => r.text)).toEqual(['Strong communication']);
  });

  it('has empty requirement groups when the ledger never arrived', () => {
    const v = viewJobMatch(resp());
    expect(v?.requirements.covered).toEqual([]);
    expect(v?.requirements.unverifiable).toEqual([]);
  });
});

describe('impactOf', () => {
  // Impact is read off the weight the SERVER sent, so a re-weighting can never leave the
  // label disagreeing with the score it labels.
  it('reads impact from the weight, heaviest first', () => {
    expect(impactOf(40)).toBe('High impact');
    expect(impactOf(30)).toBe('High impact');
    expect(impactOf(20)).toBe('Medium impact');
    expect(impactOf(10)).toBe('Low impact');
  });
});

describe('scoreTone', () => {
  it('grades a score the way the match verdict does', () => {
    expect(scoreTone(85)).toBe('strong');
    expect(scoreTone(60)).toBe('moderate');
    expect(scoreTone(20)).toBe('poor');
  });
});
