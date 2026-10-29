import { render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ProfileLayout from './+layout.svelte';

// The profile marks only what is missing: a dot on each tab holding an empty section and
// an "x/6" count while anything is left. A finished profile shows neither.

const { completeness } = vi.hoisted(() => ({ completeness: { current: null as unknown } }));

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/my/profile') } }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
vi.mock('#lib/profile.svelte.js', () => ({
  profileStore: { profile: { skills: [] }, ensureLoaded: () => Promise.resolve() },
}));
vi.mock('#lib/resume.svelte.js', () => ({
  resumeStore: { meta: null, present: false, loaded: true, ensureLoaded: () => Promise.resolve() },
}));
vi.mock('#lib/screening.svelte.js', () => ({ screeningStore: { ensureLoaded: vi.fn() } }));
vi.mock('#lib/experienceBank.svelte.js', () => ({
  experienceBankStore: { ensureLoaded: vi.fn() },
}));
vi.mock('#lib/profileCompleteness.svelte.js', () => ({
  profileCompleteness: () => completeness.current,
}));
vi.mock('#lib/components/AccountSetupCard.svelte', () => ({ default: () => {} }));

const children = createRawSnippet(() => ({ render: () => '<p>section content</p>' }));

const ALL_FILLED = {
  contacts: true,
  skills: true,
  experience: true,
  education: true,
  screening: true,
  summary: true,
  filled: 6,
  total: 6,
};

async function renderLayout() {
  render(ProfileLayout, { props: { children } });
  for (let i = 0; i < 5; i++) await tick();
}

function dotted(): string[] {
  return screen
    .getAllByRole('tab')
    .filter((tab) => tab.querySelector('[data-attention]'))
    .map((tab) => tab.getAttribute('href') ?? '');
}

beforeEach(() => {
  completeness.current = null;
});

describe('profile tab dots', () => {
  it('dots only the tabs that hold an empty section, and shows the count', async () => {
    completeness.current = { ...ALL_FILLED, education: false, screening: false, filled: 4 };
    await renderLayout();

    expect(dotted()).toEqual(['/my/profile/experience', '/my/profile/screening']);
    expect(screen.getByText('4/6 sections')).toBeTruthy();
  });

  it('shows neither a dot nor the count once everything is filled', async () => {
    completeness.current = ALL_FILLED;
    await renderLayout();

    expect(dotted()).toEqual([]);
    expect(screen.queryByText('6/6 sections')).toBeNull();
  });

  it('shows no dot while the inputs are still loading', async () => {
    await renderLayout();

    expect(dotted()).toEqual([]);
  });
});
