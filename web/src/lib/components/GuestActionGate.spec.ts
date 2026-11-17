import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, expect, it, vi } from 'vitest';

// Real components with the real gate: only navigation and the session are replaced.
const pageState = vi.hoisted(() => ({ url: new URL('http://localhost/jobs?sort=newest'), data: {} }));
const nav = vi.hoisted(() => ({ goto: vi.fn(), afterNavigate: vi.fn(), beforeNavigate: vi.fn() }));
vi.mock('$app/state', () => ({ page: pageState }));
vi.mock('$app/navigation', () => nav);
vi.mock('$app/paths', () => ({ resolve: (path: string) => path, base: '', assets: '' }));
vi.mock('#lib/auth.svelte.js', () => ({
  isAuthenticated: () => false,
  currentUser: () => null,
  logout: vi.fn(),
}));
vi.mock('#lib/api.js', () => ({ ApiError: class extends Error {}, api: { vote: vi.fn() } }));

import HeaderProfileMenu from './HeaderProfileMenu.svelte';
import VoteControl from './VoteControl.svelte';

const target = () => new URL(String(nav.goto.mock.calls.at(-1)?.[0]), 'http://localhost');

beforeEach(() => {
  nav.goto.mockClear();
  pageState.url = new URL('http://localhost/jobs?sort=newest');
});

it('a guest vote enters onboarding first and keeps the page they were on', async () => {
  render(VoteControl, { props: { target: 'job', slug: 'acme', upvoteCount: 1, downvoteCount: 0 } });
  await fireEvent.click(screen.getAllByRole('button')[0] as HTMLElement);
  expect(target().pathname).toBe('/onboarding');
  expect(target().searchParams.get('returnTo')).toBe('/jobs?sort=newest');
});

it('the explicit header Sign in stays a direct login, never an onboarding detour', async () => {
  render(HeaderProfileMenu);
  await fireEvent.click(screen.getByRole('button', { name: /sign in/i }));
  expect(target().pathname).toBe('/signin');
  expect(target().searchParams.get('mode')).toBe('login');
  expect(target().searchParams.get('returnTo')).toBe('/jobs?sort=newest');
});
