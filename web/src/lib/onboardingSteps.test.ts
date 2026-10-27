import { describe, expect, it } from 'vitest';
import { JOB_CHALLENGE_OPTIONS } from './surveyOptions';
import {
  TOUR_STEPS,
  challengeTips,
  orderSpecializations,
  plannedSteps,
  toggleChallenge,
  type TourAnswered,
} from './onboardingSteps';

const nothing: TourAnswered = { hasResume: false, hasFeed: false, hasAlert: false, hasChallenge: false };
const everything: TourAnswered = { hasResume: true, hasFeed: true, hasAlert: true, hasChallenge: true };

describe('plannedSteps', () => {
  it('gives a new account the whole tour, CV first and the two Pro steps last', () => {
    expect(plannedSteps(nothing)).toEqual(TOUR_STEPS);
    expect(TOUR_STEPS[0]).toBe('cv');
    expect(TOUR_STEPS.slice(-2)).toEqual(['tailor', 'extension']);
  });

  it('asks a returning account only what it lacks, plus the introductions', () => {
    expect(plannedSteps({ ...everything, hasAlert: false })).toEqual(['alerts', 'agent', 'tailor', 'extension']);
  });

  it('keeps the canonical order when several steps are missing', () => {
    expect(plannedSteps({ ...everything, hasFeed: false, hasChallenge: false })).toEqual([
      'feed',
      'agent',
      'challenge',
      'tailor',
      'extension',
    ]);
  });

  // The caller marks such an account complete without rendering a screen.
  it('leaves an account that lacks nothing with no steps at all', () => {
    expect(plannedSteps(everything)).toEqual([]);
  });
});

describe('orderSpecializations', () => {
  const opt = (value: string) => ({ value, label: value });
  const options = ['healthcare', 'backend', 'sales', 'devops', 'other'].map(opt);
  const tech = new Set(['backend', 'devops']);

  it('puts technical categories first, each half ordered by job count', () => {
    const counts = { healthcare: 500, sales: 300, backend: 20, devops: 60, other: 1 };
    expect(orderSpecializations(options, counts, tech).map((o) => o.value)).toEqual([
      'devops',
      'backend',
      'healthcare',
      'sales',
      'other',
    ]);
  });

  it('keeps the given order within each half when counts are not loaded yet', () => {
    expect(orderSpecializations(options, null, tech).map((o) => o.value)).toEqual([
      'backend',
      'devops',
      'healthcare',
      'sales',
      'other',
    ]);
  });

  it('does not mutate its input', () => {
    const copy = [...options];
    orderSpecializations(options, { healthcare: 9 }, tech);
    expect(options).toEqual(copy);
  });
});

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

  it('shows nothing before an answer', () => {
    expect(challengeTips([])).toEqual([]);
  });
});
