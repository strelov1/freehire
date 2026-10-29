import { render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ProfileLayout from './+layout.svelte';

// The contact editors (CV summary, contacts, education) each PUT the WHOLE contact block,
// built from the résumé store's copy. A section rendered before that copy arrives would
// save a partial block over the stored one, wiping every field it did not carry — so the
// layout must not open its gate until the résumé has loaded.

const { profileStore, resumeStore } = vi.hoisted(() => ({
  profileStore: { profile: { skills: [] } as unknown, ensureLoaded: vi.fn() },
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
  profileStore.ensureLoaded.mockReset().mockResolvedValue(undefined);
  resumeStore.ensureLoaded.mockReset();
  resumeStore.loaded = false;
});

describe('profile layout gate', () => {
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
