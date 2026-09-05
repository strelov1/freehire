import { describe, it, expect } from 'vitest';
import { accountSteps, outstandingOf, type CompletenessInput } from './accountCompleteness';
import type { UserProfile } from './types';

// What the account card measures. The predicates are the whole point of the module, so
// they are asserted one at a time: a step that silently reads as done is a funnel that
// tells someone they have finished when they have not.

const profile = (patch: Partial<UserProfile> = {}): UserProfile =>
  ({
    specializations: [],
    skills: [],
    seniorities: [],
    excluded_skills: [],
    location_preferences: null,
    derived_location: null,
    cv: null,
    created_at: null,
    updated_at: null,
    ...patch,
  }) as UserProfile;

const empty: CompletenessInput = { hasCv: false, profile: null, alertCount: 0 };

const doneIds = (input: CompletenessInput) =>
  accountSteps(input)
    .filter((s) => s.done)
    .map((s) => s.id);

describe('accountSteps', () => {
  it('reports every step outstanding for a fresh account', () => {
    const steps = accountSteps(empty);
    expect(steps.length).toBeGreaterThan(0);
    expect(steps.every((s) => !s.done)).toBe(true);
    expect(outstandingOf(steps)).toHaveLength(steps.length);
  });

  it('gives every step a label and a link to where it is done', () => {
    for (const step of accountSteps(empty)) {
      expect(step.label.trim()).not.toBe('');
      expect(step.href.startsWith('/')).toBe(true);
    }
  });

  it('counts a CV once one is stored', () => {
    expect(doneIds({ ...empty, hasCv: true })).toEqual(['cv']);
  });

  it('wants both a specialization and a seniority before the role step is done', () => {
    expect(doneIds({ ...empty, profile: profile({ specializations: ['backend'] }) })).toEqual([]);
    expect(doneIds({ ...empty, profile: profile({ seniorities: ['senior'] }) })).toEqual([]);
    expect(
      doneIds({ ...empty, profile: profile({ specializations: ['backend'], seniorities: ['senior'] }) }),
    ).toEqual(['role']);
  });

  it('counts skills once the profile names any', () => {
    expect(doneIds({ ...empty, profile: profile({ skills: ['go'] }) })).toEqual(['skills']);
  });

  // An empty preferences object is what the API returns for a user who opened the step
  // and saved nothing, so its mere presence must not read as an answer.
  it('does not count an empty location block as a stated preference', () => {
    expect(
      doneIds({
        ...empty,
        profile: profile({
          location_preferences: { remote: {}, base: {}, relocation: { open: false } },
        }),
      }),
    ).toEqual([]);
  });

  it('counts location from a work mode, a base or a remote reach', () => {
    const stated = (patch: Partial<UserProfile['location_preferences'] & object>) =>
      doneIds({
        ...empty,
        profile: profile({
          location_preferences: { remote: {}, base: {}, relocation: { open: false }, ...patch },
        }),
      });
    expect(stated({ work_modes: ['remote'] })).toEqual(['location']);
    expect(stated({ base: { country: 'PT' } })).toEqual(['location']);
    expect(stated({ remote: { countries: ['PT'] } })).toEqual(['location']);
  });

  it('counts an alert once a subscription exists', () => {
    expect(doneIds({ ...empty, alertCount: 1 })).toEqual(['alerts']);
  });

  it('reports a fully set-up account as having nothing outstanding', () => {
    const full: CompletenessInput = {
      hasCv: true,
      profile: profile({
        specializations: ['backend'],
        seniorities: ['senior'],
        skills: ['go'],
        location_preferences: { work_modes: ['remote'], remote: {}, base: {}, relocation: { open: false } },
      }),
      alertCount: 2,
    };
    expect(outstandingOf(accountSteps(full))).toEqual([]);
    expect(accountSteps(full).every((s) => s.done)).toBe(true);
  });
});
