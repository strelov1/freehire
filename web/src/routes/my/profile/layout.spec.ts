import { render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ProfileLayout from './+layout.svelte';

// The contact editors (CV summary, contacts, education) each PUT the WHOLE contact block,
// built from the résumé store's copy. A section rendered before that copy arrives would
// save a partial block over the stored one, wiping every field it did not carry — so the
// layout must not open its gate until the résumé has loaded.

const { profileStore, resumeStore } = vi.hoisted(() => ({
  profileStore: { loaded: true, profile: { skills: [] } as unknown, ensureLoaded: vi.fn(), save: vi.fn() },
  resumeStore: { meta: null, present: false, loaded: false, ensureLoaded: vi.fn() },
}));

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/my/profile') } }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
vi.mock('#lib/profile.svelte.js', () => ({ profileStore }));
vi.mock('#lib/resume.svelte.js', () => ({ resumeStore }));
vi.mock('#lib/screening.svelte.js', () => ({ screeningStore: { ensureLoaded: vi.fn() } }));
vi.mock('#lib/experienceBank.svelte.js', () => ({
  experienceBankStore: { ensureLoaded: vi.fn() },
}));
vi.mock('#lib/profileCompleteness.svelte.js', () => ({ profileCompleteness: () => null }));
vi.mock('#lib/components/AccountSetupCard.svelte', () => ({ default: () => {} }));

const children = createRawSnippet(() => ({ render: () => '<p>section content</p>' }));

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((r) => (resolve = r));
  return { promise, resolve };
}

async function settle() {
  for (let i = 0; i < 5; i++) await tick();
}

beforeEach(() => {
  profileStore.loaded = true;
  profileStore.profile = { skills: [] };
  profileStore.save.mockReset().mockResolvedValue({ skills: [], specializations: [] });
  profileStore.ensureLoaded.mockReset().mockResolvedValue(undefined);
  resumeStore.ensureLoaded.mockReset();
  resumeStore.loaded = false;
});

describe('profile layout gate', () => {
  it('does not overwrite a profile when its initial read failed', async () => {
    profileStore.profile = null;
    profileStore.loaded = false;
    resumeStore.loaded = true;
    resumeStore.ensureLoaded.mockResolvedValue(undefined);
    render(ProfileLayout, { props: { children } });
    await settle();
    expect(profileStore.save).not.toHaveBeenCalled();
    expect(screen.getByText("Couldn't load your profile.")).toBeTruthy();
  });

  it('initializes an empty profile before exposing autosaving fields', async () => {
    profileStore.profile = null;
    resumeStore.loaded = true;
    resumeStore.ensureLoaded.mockResolvedValue(undefined);
    const save = deferred();
    profileStore.save.mockReturnValue(save.promise);
    render(ProfileLayout, { props: { children } });
    await settle();
    expect(profileStore.save).toHaveBeenCalledWith([], [], [], [], [], [], null);
    expect(screen.queryByText('section content')).toBeNull();
    save.resolve();
    await settle();
    expect(screen.getByText('section content')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Create profile' })).toBeNull();
  });

  it('shows recovery when initializing the profile fails', async () => {
    profileStore.profile = null;
    resumeStore.loaded = true;
    resumeStore.ensureLoaded.mockResolvedValue(undefined);
    profileStore.save.mockRejectedValue(new Error('offline'));
    render(ProfileLayout, { props: { children } });
    await settle();
    expect(screen.getByText("Couldn't load your profile.")).toBeTruthy();
    expect(screen.queryByText('section content')).toBeNull();
  });

  it('keeps the sections hidden until the résumé has loaded', async () => {
    const resume = deferred();
    resumeStore.ensureLoaded.mockReturnValue(resume.promise);
    render(ProfileLayout, { props: { children } });
    await settle();
    expect(screen.queryByText('section content')).toBeNull();

    resumeStore.loaded = true;
    resume.resolve();
    await settle();
    expect(screen.getByText('section content')).toBeTruthy();
  });

  it('shows the load error rather than the sections when the résumé fails to load', async () => {
    // ensureLoaded swallows the failure and leaves `loaded` false.
    resumeStore.ensureLoaded.mockResolvedValue(undefined);
    render(ProfileLayout, { props: { children } });
    await settle();
    expect(screen.queryByText('section content')).toBeNull();
    expect(screen.getByText("Couldn't load your profile.")).toBeTruthy();
  });
});
