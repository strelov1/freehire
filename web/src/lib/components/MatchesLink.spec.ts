import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

let flag = '1';
let pathname = '/jobs';
vi.mock('$app/env/public', () => ({
  get PUBLIC_MATCHES() {
    return flag;
  },
}));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/state', () => ({
  page: {
    get url() {
      return new URL(`http://localhost${pathname}`);
    },
  },
}));

const { default: MatchesLink } = await import('./MatchesLink.svelte');

describe('MatchesLink', () => {
  beforeEach(() => {
    flag = '1';
    pathname = '/jobs';
  });

  it('links to /matches under the name Matches, for everyone', () => {
    render(MatchesLink);
    const link = screen.getByRole('link', { name: 'Matches', hidden: true });
    expect(link.getAttribute('href')).toBe('/matches');
    expect(link.getAttribute('aria-current')).toBeNull();
  });

  it('marks the current page', () => {
    pathname = '/matches';
    render(MatchesLink);
    expect(screen.getByRole('link', { name: 'Matches', hidden: true }).getAttribute('aria-current')).toBe('page');
  });

  it('is absent while the feed is dark', () => {
    flag = '';
    render(MatchesLink);
    expect(screen.queryByRole('link', { name: 'Matches', hidden: true })).toBeNull();
  });
});
