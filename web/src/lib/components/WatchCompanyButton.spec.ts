import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import WatchCompanyButton from './WatchCompanyButton.svelte';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
vi.mock('#lib/signin.js', () => ({ promptSignIn: vi.fn() }));
vi.mock('#lib/announce.svelte.js', () => ({ announce: vi.fn() }));

const { saved, notes } = vi.hoisted(() => ({
  saved: { items: [] as { id: number; name: string; query: string }[], ensureLoaded: vi.fn(async () => {}) },
  notes: { subscriptions: [] as { saved_search_id: number }[], ensureLoaded: vi.fn(async () => {}) },
}));
vi.mock('#lib/savedSearches.svelte.js', () => ({ savedSearches: saved }));
vi.mock('#lib/notifications.svelte.js', () => ({ notifications: notes }));
vi.mock('#lib/api.js', () => ({
  ApiError: class extends Error {},
  api: { isWatched: vi.fn(async () => false), watchCompany: vi.fn(async () => {}), unwatchCompany: vi.fn(async () => {}) },
}));

describe('WatchCompanyButton', () => {
  beforeEach(() => {
    saved.items = [];
    notes.subscriptions = [];
  });

  it('points to the watchlist alerts after a Watch when none is on', async () => {
    render(WatchCompanyButton, { slug: 'acme', companyName: 'Acme' });
    await fireEvent.click(screen.getByRole('button', { name: 'Watch' }));
    const link = await screen.findByRole('link', { name: 'Turn on alerts' });
    expect(link.getAttribute('href')).toBe('/my/watchlist');
  });

  it('says nothing more when the watchlist alert already delivers', async () => {
    saved.items = [{ id: 7, name: 'Companies I follow', query: 'watchlist=true' }];
    notes.subscriptions = [{ saved_search_id: 7 }];
    render(WatchCompanyButton, { slug: 'acme', companyName: 'Acme' });
    await fireEvent.click(screen.getByRole('button', { name: 'Watch' }));
    await screen.findByRole('button', { name: 'Watching' });
    expect(screen.queryByRole('link', { name: 'Turn on alerts' })).toBeNull();
  });
});
