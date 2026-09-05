import { describe, expect, it } from 'vitest';
import { ORDERED_STEPS, plannedSteps, type OnboardingAnswered } from './onboardingSteps';

/** Nothing answered — the state a freshly registered account is in. */
const nothing: OnboardingAnswered = {
  hasResume: false,
  hasSpecializations: false,
  hasProfileLinks: false,
  hasTotalYears: false,
  hasSkills: false,
  hasLocation: false,
  hasDesiredSalary: false,
  hasCurrentIncome: false,
  hasStage: false,
  hasChallenge: false,
};

describe('plannedSteps', () => {
  it('gives a fresh account every step, in order', () => {
    expect(plannedSteps(nothing)).toEqual(ORDERED_STEPS);
  });

  it('puts the acted-on questions before the ones that only describe the candidate', () => {
    // An abandoned run should have answered the things that change what the user is shown
    // before the things that only tell us about them.
    const order = plannedSteps(nothing);
    for (const research of ['stage', 'challenge'] as const) {
      for (const product of ['cv', 'confirm', 'experience', 'skills', 'location', 'money'] as const) {
        expect(order.indexOf(product), `${product} before ${research}`).toBeLessThan(order.indexOf(research));
      }
    }
  });

  it('drops a step whose answer is already stored', () => {
    expect(plannedSteps({ ...nothing, hasResume: true })).not.toContain('cv');
    expect(plannedSteps({ ...nothing, hasStage: true })).not.toContain('stage');
  });

  it('keeps asking the confirm step until BOTH its questions are answered', () => {
    // Specializations and profile links share one screen. Treating it as done on the
    // specializations alone would mean an existing account is never once asked for its
    // LinkedIn — and the wizard only runs once, so never means never.
    expect(plannedSteps({ ...nothing, hasSpecializations: true })).toContain('confirm');
    expect(plannedSteps({ ...nothing, hasProfileLinks: true })).toContain('confirm');
    expect(plannedSteps({ ...nothing, hasSpecializations: true, hasProfileLinks: true })).not.toContain('confirm');
  });

  it('keeps asking the money step until BOTH its figures are stored', () => {
    // Same rule as the confirm step, for the same reason: the screen carries two
    // independent questions (desired salary in the screening answers, current income in the
    // survey), and any account that ever set a desired salary through /me/screening-answers
    // would otherwise never once be asked what it earns today.
    expect(plannedSteps({ ...nothing, hasDesiredSalary: true })).toContain('money');
    expect(plannedSteps({ ...nothing, hasCurrentIncome: true })).toContain('money');
    expect(plannedSteps({ ...nothing, hasDesiredSalary: true, hasCurrentIncome: true })).not.toContain('money');
  });

  it('leaves an account that has answered everything with no steps at all', () => {
    const everything: OnboardingAnswered = {
      hasResume: true,
      hasSpecializations: true,
      hasProfileLinks: true,
      hasTotalYears: true,
      hasSkills: true,
      hasLocation: true,
      hasDesiredSalary: true,
      hasCurrentIncome: true,
      hasStage: true,
      hasChallenge: true,
    };
    expect(plannedSteps(everything)).toEqual([]);
  });

  it('keeps the surviving steps in their canonical order', () => {
    // A trimmed plan must not reshuffle: the money step still comes after skills for the
    // existing account that only skipped the CV.
    const got = plannedSteps({ ...nothing, hasResume: true, hasSkills: true });
    expect(got).toEqual(ORDERED_STEPS.filter((s) => s !== 'cv' && s !== 'skills'));
  });
});
