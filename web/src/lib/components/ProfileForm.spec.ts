import { fireEvent, render } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { UserProfile } from '#lib/types.js';
import ProfileForm from './ProfileForm.svelte';

// Today's fixture always passes a non-null `profile`, so the `{#if !editing}` branch
// (SkillsPicker + LocationPreferencesFields, both now locale()-aware) never mounts —
// but it exists in this file, and without this mock the shared `$app/state` stub
// (only `{ url }`, no `data`) would throw the moment a future test exercises it.
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/') } }));

const { extractResumeProfile, getPhoto, mergeResumeExtraction, profile } = vi.hoisted(() => ({
  extractResumeProfile: vi.fn(),
  getPhoto: vi.fn(),
  mergeResumeExtraction: vi.fn(),
  profile: {
    specializations: ['backend'],
    skills: ['go'],
    seniorities: [],
    excluded_skills: [],
    excluded_sources: [],
    excluded_companies: [],
    location_preferences: null,
    derived_location: null,
    cv: null,
    created_at: null,
    updated_at: null,
  } as UserProfile,
}));

vi.mock('#lib/api.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('#lib/api.js')>();
  return { ...actual, api: { ...actual.api, extractResumeProfile, getPhoto } };
});

vi.mock('#lib/profile.svelte.js', () => ({
  profileStore: {
    get profile() {
      return profile;
    },
    mergeResumeExtraction,
  },
}));

function uploadResume(file: File) {
  const input = document.querySelector('input[type="file"]') as HTMLInputElement;
  Object.defineProperty(input, 'files', { value: [file], configurable: true });
  return fireEvent.change(input);
}

beforeEach(() => {
  getPhoto.mockReset().mockRejectedValue(new Error('no photo'));
  mergeResumeExtraction.mockReset().mockResolvedValue(profile);
});

describe('ProfileForm', () => {
  it('calls onSaved after a CV re-upload merges new skills into an existing profile', async () => {
    extractResumeProfile.mockReset().mockResolvedValue({ skills: ['rust'], categories: [] });
    const onSaved = vi.fn();
    render(ProfileForm, { props: { profile, hasCv: false, onSaved } });

    await uploadResume(new File(['pdf'], 'cv.pdf', { type: 'application/pdf' }));
    await vi.waitFor(() => expect(mergeResumeExtraction).toHaveBeenCalledWith(['rust'], ['backend']));

    expect(onSaved).toHaveBeenCalledTimes(1);
  });

  it('does not call onSaved when the extraction adds nothing new', async () => {
    // The profile already has 'go'; extracting the same skill again adds nothing, so
    // analyzeResume's `editing` branch never reaches profileStore.mergeResumeExtraction.
    extractResumeProfile.mockReset().mockResolvedValue({ skills: ['go'], categories: [] });
    const onSaved = vi.fn();
    render(ProfileForm, { props: { profile, hasCv: false, onSaved } });

    await uploadResume(new File(['pdf'], 'cv.pdf', { type: 'application/pdf' }));
    await vi.waitFor(() => expect(extractResumeProfile).toHaveBeenCalled());

    expect(mergeResumeExtraction).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
  });
});
