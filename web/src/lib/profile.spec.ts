import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { UserProfile } from '#lib/types.js';
import { profileStore } from './profile.svelte';

const { saveProfile } = vi.hoisted(() => ({ saveProfile: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { saveProfile } }));

const emptyProfile: UserProfile = {
  specializations: [], skills: [], seniorities: [],
  excluded_skills: [], excluded_sources: [], excluded_companies: [],
  location_preferences: null, derived_location: null, cv: null,
  created_at: null, updated_at: null,
};

beforeEach(async () => {
  profileStore.reset();
  saveProfile.mockReset().mockImplementation(async (
    specializations, skills, seniorities, excluded_skills,
    excluded_sources, excluded_companies, location_preferences,
  ) => ({
    ...emptyProfile, specializations, skills, seniorities, excluded_skills,
    excluded_sources, excluded_companies, location_preferences,
  }));
  await profileStore.save(['backend'], ['go'], [], [], [], [], null);
  saveProfile.mockClear();
});

describe('profile CV merge', () => {
  it('preserves a manual role edit queued before the CV import', async () => {
    let finish!: (profile: UserProfile) => void;
    saveProfile.mockImplementationOnce(() => new Promise<UserProfile>((resolve) => { finish = resolve; }));
    const edit = profileStore.updateSpecializations(['backend', 'devops']);
    await vi.waitFor(() => expect(saveProfile).toHaveBeenCalledOnce());

    const importing = profileStore.mergeResumeExtraction(['rust'], ['frontend']);
    finish({ ...emptyProfile, specializations: ['backend', 'devops'], skills: ['go'] });
    await edit;
    const result = await importing;

    expect(profileStore.profile?.specializations).toEqual(['backend', 'devops', 'frontend']);
    expect(profileStore.profile?.skills).toEqual(['go', 'rust']);
    expect(result).toEqual({ addedSkills: 1, addedSpecializations: 1, droppedSpecializations: 0 });
  });

  it('does not write when an extraction adds nothing', async () => {
    const result = await profileStore.mergeResumeExtraction(['go'], ['backend']);
    expect(saveProfile).not.toHaveBeenCalled();
    expect(result).toEqual({ addedSkills: 0, addedSpecializations: 0, droppedSpecializations: 0 });
  });

  it('caps imported roles while preserving every manually selected role', async () => {
    const roles = ['backend', 'frontend', 'fullstack', 'mobile', 'devops', 'sre', 'qa', 'security', 'hardware', 'embedded'];
    await profileStore.updateSpecializations(roles);
    saveProfile.mockClear();

    const result = await profileStore.mergeResumeExtraction(['rust'], ['blockchain']);
    expect(profileStore.profile?.specializations).toEqual(roles);
    expect(profileStore.profile?.skills).toEqual(['go', 'rust']);
    expect(result).toEqual({ addedSkills: 1, addedSpecializations: 0, droppedSpecializations: 1 });
  });
});
