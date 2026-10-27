import { describe, expect, it, vi } from 'vitest';
import { TOUR_STEPS } from './onboardingSteps';
import { persistStep, type SaveDeps, type TourAnswers } from './onboardingSave';

function answers(over: Partial<TourAnswers> = {}): TourAnswers {
  return {
    specializations: [],
    skills: [],
    seniorities: [],
    excludedSkills: [],
    excludedSources: [],
    excludedCompanies: [],
    location: null,
    challenges: [],
    challengeNote: '',
    ...over,
  };
}

function deps(): SaveDeps & { surveyPatches: unknown[]; profileCalls: unknown[][] } {
  const surveyPatches: unknown[] = [];
  const profileCalls: unknown[][] = [];
  return {
    surveyPatches,
    profileCalls,
    saveProfile: vi.fn(async (...args: unknown[]) => {
      profileCalls.push(args);
    }),
    updateSurvey: vi.fn(async (p) => {
      surveyPatches.push(p);
    }),
  };
}

describe('the feed step', () => {
  it('saves the whole search profile once a specialization and a skill exist', async () => {
    const d = deps();
    const location = { remote: { regions: [], countries: [] }, base: { cities: [] }, relocation: {} };
    await persistStep(
      'feed',
      answers({
        specializations: ['backend'],
        skills: ['go'],
        seniorities: ['senior'],
        excludedSkills: ['php'],
        location: location as unknown as TourAnswers['location'],
      }),
      d,
    );
    expect(d.profileCalls).toEqual([[['backend'], ['go'], ['senior'], ['php'], [], [], location]]);
  });

  // The endpoint rejects either being empty, so there is nowhere for a level-only save to
  // land. Skipping beats failing: blocking here would make an optional step mandatory.
  it('skips the save when the profile could not exist yet', async () => {
    const d = deps();
    await persistStep('feed', answers({ seniorities: ['senior'] }), d);
    await persistStep('feed', answers({ specializations: ['backend'] }), d);
    expect(d.profileCalls).toHaveLength(0);
  });
});

describe('the challenge step', () => {
  it('sends every picked answer, and the note only while "other" is among them', async () => {
    const d = deps();
    await persistStep('challenge', answers({ challenges: ['english', 'other'], challengeNote: ' visas ' }), d);
    expect(d.surveyPatches).toEqual([{ biggest_challenges: ['english', 'other'], biggest_challenge_note: 'visas' }]);
  });

  // The server rejects a note beside a coded challenge, so sending a leftover one would turn
  // a valid answer into a 400.
  it('drops a leftover note when the challenge is a coded one', async () => {
    const d = deps();
    await persistStep('challenge', answers({ challenges: ['english'], challengeNote: 'visas' }), d);
    expect(d.surveyPatches).toEqual([{ biggest_challenges: ['english'] }]);
  });

  it('writes nothing before an answer', async () => {
    const d = deps();
    await persistStep('challenge', answers(), d);
    expect(d.surveyPatches).toHaveLength(0);
  });
});

describe('the steps that only show a feature', () => {
  it('write nothing, even with every answer present', async () => {
    const d = deps();
    const full = answers({ specializations: ['backend'], skills: ['go'], challenges: ['english'] });
    for (const step of TOUR_STEPS.filter((s) => s !== 'feed' && s !== 'challenge')) {
      await persistStep(step, full, d);
    }
    expect(d.profileCalls).toHaveLength(0);
    expect(d.surveyPatches).toHaveLength(0);
  });
});
