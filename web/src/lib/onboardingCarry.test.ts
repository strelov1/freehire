import { describe, expect, it, vi } from 'vitest';

import { carryFailureMessage, carryInto, type CarryDeps, type CarryInput } from './onboardingCarry';
import type { OnboardingParse, UserProfile } from './types';

const input: CarryInput = {
  file: new Blob(['%PDF'], { type: 'application/pdf' }),
  fileName: 'cv.pdf',
  parse: {
    skills: ['go'],
    categories: ['backend'],
    seniority: 'senior',
    country: 'br',
  } as unknown as OnboardingParse,
  answers: { where: 'remote', role: 'backend', challenge: [] },
};

function deps(over: Partial<CarryDeps> = {}): CarryDeps {
  return {
    uploadCv: vi.fn(() => Promise.resolve()),
    saveProfile: vi.fn((d) => Promise.resolve({ ...d, saved: true } as unknown as UserProfile)),
    enableAlert: vi.fn(() => Promise.resolve()),
    completeOnboarding: vi.fn(() => Promise.resolve()),
    saveSurvey: vi.fn(() => Promise.resolve()),
    ...over,
  };
}

describe('carryInto', () => {
  it('runs every step and reports nothing when all succeed', async () => {
    const d = deps();
    expect(await carryInto(input, d)).toEqual([]);
    const file = vi.mocked(d.uploadCv).mock.calls[0]?.[0];
    expect(file).toBeInstanceOf(File);
    expect(file?.name).toBe('cv.pdf');
    expect(vi.mocked(d.saveProfile).mock.calls[0]?.[0]).toMatchObject({
      specializations: ['backend'],
      seniorities: ['senior'],
      skills: ['go'],
    });
    // The alert is built from the profile as the server saved it.
    expect(vi.mocked(d.enableAlert).mock.calls[0]?.[0]).toMatchObject({ saved: true });
    expect(d.completeOnboarding).toHaveBeenCalled();
  });

  it('still runs the other steps when one fails, and names it', async () => {
    const d = deps({ enableAlert: vi.fn(() => Promise.reject(new Error('no'))) });
    expect(await carryInto(input, d)).toEqual(['alert']);
    expect(d.uploadCv).toHaveBeenCalled();
    expect(d.saveProfile).toHaveBeenCalled();
    expect(d.completeOnboarding).toHaveBeenCalled();
  });

  it('subscribes the alert from the answers when the profile save failed', async () => {
    const d = deps({ saveProfile: vi.fn(() => Promise.reject(new Error('400'))) });
    expect(await carryInto(input, d)).toEqual(['profile']);
    expect(vi.mocked(d.enableAlert).mock.calls[0]?.[0]).toMatchObject({ specializations: ['backend'] });
  });

  it('runs only the steps asked for on a retry', async () => {
    const d = deps();
    expect(await carryInto(input, d, ['cv'])).toEqual([]);
    expect(d.uploadCv).toHaveBeenCalled();
    expect(d.saveProfile).not.toHaveBeenCalled();
    expect(d.enableAlert).not.toHaveBeenCalled();
  });

  it('does not report a failed completion mark, which only re-offers the flow', async () => {
    const d = deps({ completeOnboarding: vi.fn(() => Promise.reject(new Error('503'))) });
    expect(await carryInto(input, d)).toEqual([]);
  });
});

describe('the survey step', () => {
  const withChallenge = (challenge: string[], challengeNote?: string): CarryInput => ({
    ...input,
    answers: { ...input.answers, challenge, challengeNote },
  });

  it('saves what was picked', async () => {
    const d = deps();
    expect(await carryInto(withChallenge(['visa_sponsorship', 'english']), d)).toEqual([]);
    expect(d.saveSurvey).toHaveBeenCalledWith({ biggest_challenges: ['visa_sponsorship', 'english'] });
  });

  // The server takes a note only beside `other`.
  it('sends the note only beside "something else"', async () => {
    const d = deps();
    await carryInto(withChallenge(['other'], '  Too many tabs '), d);
    expect(d.saveSurvey).toHaveBeenCalledWith({
      biggest_challenges: ['other'],
      biggest_challenge_note: 'Too many tabs',
    });
    const e = deps();
    await carryInto(withChallenge(['english'], 'stale note'), e);
    expect(e.saveSurvey).toHaveBeenCalledWith({ biggest_challenges: ['english'] });
  });

  it('is skipped when nothing was picked', async () => {
    const d = deps();
    await carryInto(withChallenge([]), d);
    expect(d.saveSurvey).not.toHaveBeenCalled();
  });

  it('reports its failure and leaves the other steps alone', async () => {
    const d = deps({ saveSurvey: vi.fn(() => Promise.reject(new Error('400'))) });
    expect(await carryInto(withChallenge(['english']), d)).toEqual(['survey']);
    expect(d.uploadCv).toHaveBeenCalled();
    expect(d.enableAlert).toHaveBeenCalled();
  });
});

describe('carryFailureMessage', () => {
  it('names what did not save', () => {
    expect(carryFailureMessage(['alert'])).toBe("Your account is ready, but we couldn't save the email alert.");
    expect(carryFailureMessage(['cv', 'profile'])).toBe(
      "Your account is ready, but we couldn't save your CV and your search profile.",
    );
    expect(carryFailureMessage(['survey'])).toBe(
      "Your account is ready, but we couldn't save your answers about what's in your way.",
    );
  });
});
