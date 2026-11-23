import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { UserProfile } from '#lib/types.js';
import { profileStore } from './profile.svelte';

const { saveProfile, excludeCompany, restoreCompany, syncProfileAlert } = vi.hoisted(() => ({
  saveProfile: vi.fn(),
  excludeCompany: vi.fn(),
  restoreCompany: vi.fn(),
  syncProfileAlert: vi.fn().mockResolvedValue(undefined),
}));
vi.mock('#lib/api.js', () => ({ api: { saveProfile, excludeCompany, restoreCompany } }));
vi.mock('#lib/profileAlertSync.js', () => ({ syncProfileAlert }));

const emptyProfile: UserProfile = {
  specializations: [],
  skills: [],
  seniorities: [],
  excluded_skills: [],
  excluded_sources: [],
  excluded_companies: [],
  job_languages: [],
  location_preferences: null,
  derived_location: null,
  cv: null,
  created_at: null,
  updated_at: null,
};

beforeEach(async () => {
  profileStore.reset();
  saveProfile
    .mockReset()
    .mockImplementation(
      async (
        specializations,
        skills,
        seniorities,
        excluded_skills,
        excluded_sources,
        location_preferences,
        job_languages,
      ) => ({
        ...emptyProfile,
        specializations,
        skills,
        seniorities,
        excluded_skills,
        excluded_sources,
        location_preferences,
        job_languages,
      }),
    );
  await profileStore.save(['backend'], ['go'], [], [], [], null);
  saveProfile.mockClear();
});

describe('profile CV merge', () => {
  it('preserves a manual role edit queued before the CV import', async () => {
    let finish!: (profile: UserProfile) => void;
    saveProfile.mockImplementationOnce(
      () =>
        new Promise<UserProfile>((resolve) => {
          finish = resolve;
        }),
    );
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
    const roles = [
      'backend',
      'frontend',
      'fullstack',
      'mobile',
      'devops',
      'sre',
      'qa',
      'security',
      'hardware',
      'embedded',
    ];
    await profileStore.updateSpecializations(roles);
    saveProfile.mockClear();

    const result = await profileStore.mergeResumeExtraction(['rust'], ['blockchain']);
    expect(profileStore.profile?.specializations).toEqual(roles);
    expect(profileStore.profile?.skills).toEqual(['go', 'rust']);
    expect(result).toEqual({ addedSkills: 1, addedSpecializations: 0, droppedSpecializations: 1 });
  });
});

describe('profile job language saves', () => {
  it('preserves languages while skills, roles and location change', async () => {
    await profileStore.updateJobLanguages(['en', 'ru']);
    await profileStore.addSkill('rust');
    await profileStore.updateSpecializations(['devops']);
    await profileStore.updateLocation(null);
    expect(profileStore.profile?.job_languages).toEqual(['en', 'ru']);
    expect(profileStore.profile?.skills).toEqual(['go', 'rust']);
    expect(profileStore.profile?.specializations).toEqual(['devops']);
  });

  it('clears languages without discarding the rest of the profile', async () => {
    await profileStore.updateJobLanguages(['en']);
    await profileStore.updateJobLanguages([]);
    expect(profileStore.profile?.job_languages).toEqual([]);
    expect(profileStore.profile?.skills).toEqual(['go']);
  });

  it('keeps the saved languages if the update fails', async () => {
    await profileStore.updateJobLanguages(['en']);
    saveProfile.mockRejectedValueOnce(new Error('offline'));
    await expect(profileStore.updateJobLanguages(['ru'])).rejects.toThrow('offline');
    expect(profileStore.profile?.job_languages).toEqual(['en']);
  });
});

describe('companies to avoid', () => {
  beforeEach(() => {
    excludeCompany.mockReset();
    restoreCompany.mockReset();
    syncProfileAlert.mockClear();
  });

  it('hides through its own endpoint, applies the reply and syncs the alert', async () => {
    excludeCompany.mockResolvedValue({ ...emptyProfile, skills: ['go'], excluded_companies: ['acme'] });
    await profileStore.avoidCompany('acme');
    expect(excludeCompany).toHaveBeenCalledWith('acme');
    expect(saveProfile).not.toHaveBeenCalled();
    expect(profileStore.profile?.excluded_companies).toEqual(['acme']);
    expect(syncProfileAlert).toHaveBeenCalledOnce();
  });

  it('leaves the profile and the alert alone when the list is full (409)', async () => {
    excludeCompany.mockRejectedValue(new Error('full'));
    const before = profileStore.profile;
    await expect(profileStore.avoidCompany('acme')).rejects.toThrow('full');
    expect(profileStore.profile).toBe(before);
    expect(syncProfileAlert).not.toHaveBeenCalled();
  });

  it('restores through its own endpoint and keeps the reply', async () => {
    restoreCompany.mockResolvedValue({ ...emptyProfile, skills: ['go'], excluded_companies: [] });
    await profileStore.unavoidCompany('acme');
    expect(restoreCompany).toHaveBeenCalledWith('acme');
    expect(profileStore.profile?.excluded_companies).toEqual([]);
    expect(syncProfileAlert).toHaveBeenCalledOnce();
  });

  it('drops old-session replies and queued exclusions after sign-out', async () => {
    let finish!: (p: UserProfile) => void;
    excludeCompany.mockReturnValue(new Promise<UserProfile>((resolve) => (finish = resolve)));
    const hiding = profileStore.avoidCompany('acme');
    await vi.waitFor(() => expect(excludeCompany).toHaveBeenCalled());
    const queued = expect(profileStore.avoidCompany('other')).rejects.toThrow('The session changed.');
    profileStore.reset();
    finish({ ...emptyProfile, excluded_companies: ['acme'] });
    await hiding;
    await queued;
    expect(excludeCompany).toHaveBeenCalledOnce();
    expect(syncProfileAlert).not.toHaveBeenCalled();
    expect(profileStore.profile).toBeNull();
  });
});
