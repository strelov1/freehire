import { describe, expect, it } from 'vitest';

import { challengeChip, challengeTips, MAX_TIPS, toggleChallenge } from './onboardingChallenge';
import { JOB_CHALLENGE_OPTIONS } from './surveyOptions';

describe('toggleChallenge', () => {
  it('adds and removes an answer, keeping the order picked', () => {
    expect(toggleChallenge(['english'], 'job_fit')).toEqual(['english', 'job_fit']);
    expect(toggleChallenge(['english', 'job_fit'], 'english')).toEqual(['job_fit']);
  });

  it('lets "just starting" stand alone, whichever comes second', () => {
    expect(toggleChallenge(['english', 'job_fit'], 'just_starting')).toEqual(['just_starting']);
    expect(toggleChallenge(['just_starting'], 'english')).toEqual(['english']);
  });
});

describe('challengeTips', () => {
  it('answers getting replies with tailoring, form filling, referrals and tracking', () => {
    expect(challengeTips(['recruiter_contact']).map((t) => t.title)).toEqual([
      'A CV for each job',
      'Forms filled in for you',
      'Anonymous employee referrals',
      'Track every application',
    ]);
  });

  it('sends visa sponsorship to the filter and the register companies', () => {
    const tips = challengeTips(['visa_sponsorship']);
    expect(tips.map((t) => t.href)).toEqual(['/jobs?visa_sponsorship=true', '/collections/visa-sponsors']);
  });

  it('shows a tip shared by two answers once', () => {
    const titles = challengeTips(['remote_flexible', 'working_abroad']).map((t) => t.title);
    expect(new Set(titles).size).toBe(titles.length);
  });

  it('has tips for every answer', () => {
    for (const { value } of JOB_CHALLENGE_OPTIONS) {
      expect(challengeTips([value]).length).toBeGreaterThan(0);
    }
  });

  it(`shows at most ${MAX_TIPS}`, () => {
    expect(challengeTips(['recruiter_contact', 'technical_interviews', 'visa_sponsorship'])).toHaveLength(MAX_TIPS);
  });

  it('shows nothing before an answer', () => {
    expect(challengeTips([])).toEqual([]);
  });
});

describe('challengeChip', () => {
  it('names the first answer and counts the rest', () => {
    expect(challengeChip(['visa_sponsorship'])).toBe('Getting visa sponsorship');
    expect(challengeChip(['visa_sponsorship', 'english', 'job_fit'])).toBe('Getting visa sponsorship +2');
  });

  it('says nothing for none', () => {
    expect(challengeChip([])).toBeNull();
  });
});
