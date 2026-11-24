import { render, screen } from '@testing-library/svelte';
import { expect, it, vi } from 'vitest';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/matches') } }));

const { default: MatchesSignedOut } = await import('./MatchesSignedOut.svelte');

it('explains the feed and signs up back to /matches', () => {
  render(MatchesSignedOut);
  expect(screen.getByRole('heading', { name: 'Jobs matched to you, one at a time' })).toBeTruthy();
  expect(screen.getAllByRole('listitem')).toHaveLength(3);
  expect(screen.getByRole('link', { name: 'Sign up' }).getAttribute('href')).toBe('/signin?returnTo=%2Fmatches');
});
