import { fireEvent, render } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ProfileForm from './ProfileForm.svelte';

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/') } }));

const { extractResumeProfile, getPhoto, mergeResumeExtraction } = vi.hoisted(() => ({
  extractResumeProfile: vi.fn(),
  getPhoto: vi.fn(),
  mergeResumeExtraction: vi.fn(),
}));

vi.mock('#lib/api.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('#lib/api.js')>();
  return { ...actual, api: { ...actual.api, extractResumeProfile, getPhoto } };
});

vi.mock('#lib/profile.svelte.js', () => ({
  profileStore: {
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
  mergeResumeExtraction.mockReset().mockResolvedValue({ addedSkills: 1, addedSpecializations: 0, droppedSpecializations: 0 });
});

describe('ProfileForm', () => {
  it('calls onSaved after a CV re-upload merges new skills into an existing profile', async () => {
    extractResumeProfile.mockReset().mockResolvedValue({ skills: ['rust'], categories: [] });
    const onSaved = vi.fn();
    render(ProfileForm, { props: { hasCv: false, onSaved } });

    await uploadResume(new File(['pdf'], 'cv.pdf', { type: 'application/pdf' }));
    await vi.waitFor(() => expect(mergeResumeExtraction).toHaveBeenCalledWith(['rust'], []));

    expect(onSaved).toHaveBeenCalledTimes(1);
  });

  it('does not call onSaved when the extraction adds nothing new', async () => {
    extractResumeProfile.mockReset().mockResolvedValue({ skills: ['go'], categories: [] });
    mergeResumeExtraction.mockResolvedValue({ addedSkills: 0, addedSpecializations: 0, droppedSpecializations: 0 });
    const onSaved = vi.fn();
    render(ProfileForm, { props: { hasCv: false, onSaved } });

    await uploadResume(new File(['pdf'], 'cv.pdf', { type: 'application/pdf' }));
    await vi.waitFor(() => expect(extractResumeProfile).toHaveBeenCalled());

    expect(mergeResumeExtraction).toHaveBeenCalledWith(['go'], []);
    expect(onSaved).not.toHaveBeenCalled();
  });
});
