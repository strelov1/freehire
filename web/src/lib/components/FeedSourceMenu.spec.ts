import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const listSavedSearches = vi.fn();
const setFeedSource = vi.fn();
vi.mock('#lib/api.js', () => ({
  api: {
    listSavedSearches: (...a: unknown[]) => listSavedSearches(...a),
    setFeedSource: (...a: unknown[]) => setFeedSource(...a),
  },
}));
vi.mock('#lib/announce.svelte.js', () => ({ announce: vi.fn() }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

const { default: FeedSourceMenu } = await import('./FeedSourceMenu.svelte');
const { feedSource } = await import('#lib/feedSource.svelte.js');

const search = (id: number, name: string, derived = false) => ({
  id,
  name,
  query: 'q=go',
  derived_from_profile: derived,
  created_at: null,
  updated_at: null,
});

describe('FeedSourceMenu', () => {
  beforeEach(() => {
    listSavedSearches.mockReset();
    setFeedSource.mockReset();
    setFeedSource.mockResolvedValue(undefined);
    feedSource.sync({ source: 'profile', total: 0 });
    feedSource.version = 0;
  });

  it('lists the profile and the saved searches, never the profile-derived one', async () => {
    listSavedSearches.mockResolvedValue([search(2, 'Go remote'), search(3, 'Alert', true)]);
    render(FeedSourceMenu);
    await fireEvent.click(screen.getByRole('button', { name: 'Match source: By my profile' }));
    expect(await screen.findByRole('menuitemradio', { name: 'Go remote' })).toBeTruthy();
    expect(screen.queryByRole('menuitemradio', { name: 'Alert' })).toBeNull();
    expect(screen.getByRole('menuitemradio', { name: 'By my profile' }).getAttribute('aria-checked')).toBe('true');
  });

  it('picks a search, stores it and bumps the feed version', async () => {
    listSavedSearches.mockResolvedValue([search(2, 'Go remote')]);
    render(FeedSourceMenu);
    await fireEvent.click(screen.getByRole('button', { name: /Match source/ }));
    await fireEvent.click(await screen.findByRole('menuitemradio', { name: 'Go remote' }));
    expect(setFeedSource).toHaveBeenCalledWith({ source: 'saved_search', saved_search_id: 2 });
    await vi.waitFor(() => expect(feedSource.version).toBe(1));
    expect(screen.getByRole('button', { name: 'Match source: Go remote' })).toBeTruthy();
  });

  it('offers Create a saved search when there are none', async () => {
    listSavedSearches.mockResolvedValue([]);
    render(FeedSourceMenu);
    await fireEvent.click(screen.getByRole('button', { name: /Match source/ }));
    const link = await screen.findByRole('menuitem', { name: 'Create a saved search' });
    expect(link.getAttribute('href')).toBe('/jobs');
  });

  it('shows a one-line status when the searches fail to load', async () => {
    listSavedSearches.mockRejectedValue(new Error('boom'));
    render(FeedSourceMenu);
    await fireEvent.click(screen.getByRole('button', { name: /Match source/ }));
    expect(await screen.findByText("Couldn't load your saved searches.")).toBeTruthy();
  });

  it('is a menu button whose keys come from menuNav', async () => {
    listSavedSearches.mockResolvedValue([]);
    render(FeedSourceMenu);
    const pill = screen.getByRole('button', { name: /Match source/ });
    expect(pill.getAttribute('aria-haspopup')).toBe('menu');
    expect(pill.getAttribute('aria-expanded')).toBe('false');
    await fireEvent.click(pill);
    expect(pill.getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByRole('menu')).toBeTruthy();
    await fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' });
    expect(screen.queryByRole('menu')).toBeNull();
  });
});
