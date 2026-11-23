import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
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
const { default: FeedCaughtUp } = await import('./FeedCaughtUp.svelte');
const { feedSource } = await import('#lib/feedSource.svelte.js');
const { resetUserStores } = await import('#lib/userResource.svelte.js');

const search = (id: number, name: string, derived = false) => ({
  id,
  name,
  query: 'q=go',
  derived_from_profile: derived,
});

describe('FeedSourceMenu', () => {
  beforeEach(() => {
    listSavedSearches.mockReset();
    setFeedSource.mockReset();
    setFeedSource.mockResolvedValue(undefined);
    feedSource.reset();
    feedSource.current = { kind: 'profile' };
    feedSource.version = 0;
  });

  it('lists the profile and own searches, never the profile-derived one, with the current checked', async () => {
    listSavedSearches.mockResolvedValue([search(1, 'Go remote'), search(9, 'Alert', true)]);
    render(FeedSourceMenu);
    const pill = screen.getByRole('button', { name: 'Match source: By my profile' });
    await fireEvent.click(pill);
    const items = await screen.findAllByRole('menuitemradio');
    expect(items.map((i) => i.textContent?.trim())).toEqual(['By my profile', 'Go remote']);
    expect(items[0]?.getAttribute('aria-checked')).toBe('true');
    expect(items[1]?.getAttribute('aria-checked')).toBe('false');
  });

  it('stores the pick, restarts the feed and returns focus to the pill', async () => {
    listSavedSearches.mockResolvedValue([search(1, 'Go remote')]);
    render(FeedSourceMenu);
    const pill = screen.getByRole('button', { name: /Match source/ });
    await fireEvent.click(pill);
    await fireEvent.click(await screen.findByText('Go remote'));
    expect(setFeedSource).toHaveBeenCalledWith({ source: 'saved_search', saved_search_id: 1 });
    await waitFor(() => expect(feedSource.version).toBe(1));
    await waitFor(() => expect(document.activeElement).toBe(pill));
    expect(pill.getAttribute('aria-label')).toBe('Match source: Go remote');
  });

  it('offers Create a saved search when there are none', async () => {
    listSavedSearches.mockResolvedValue([]);
    render(FeedSourceMenu);
    await fireEvent.click(screen.getByRole('button', { name: /Match source/ }));
    expect((await screen.findByText('Create a saved search')).getAttribute('href')).toBe('/jobs');
  });

  it('shows a one-line status when the searches fail to load, and keeps the profile item', async () => {
    listSavedSearches.mockRejectedValue(new Error('boom'));
    render(FeedSourceMenu);
    await fireEvent.click(screen.getByRole('button', { name: /Match source/ }));
    expect(await screen.findByText("Couldn't load your searches.")).toBeTruthy();
    expect(screen.getByRole('menuitemradio', { name: 'By my profile' })).toBeTruthy();
  });

  it('retries a failed search load when reopened', async () => {
    const pending = Promise.withResolvers<ReturnType<typeof search>[]>();
    listSavedSearches.mockReturnValueOnce(pending.promise).mockResolvedValueOnce([search(1, 'Go remote')]);
    render(FeedSourceMenu);
    const pill = screen.getByRole('button', { name: /Match source/ });
    await fireEvent.click(pill);
    pending.reject(new Error('temporarily unavailable'));
    expect(await screen.findByText("Couldn't load your searches.")).toBeTruthy();
    await fireEvent.click(pill);
    await fireEvent.click(pill);
    expect(await screen.findByRole('menuitemradio', { name: 'Go remote' })).toBeTruthy();
  });

  it('keeps the source and says so when the server refuses the pick', async () => {
    listSavedSearches.mockResolvedValue([search(1, 'Go remote')]);
    setFeedSource.mockRejectedValue(new Error('404'));
    render(FeedSourceMenu);
    await fireEvent.click(screen.getByRole('button', { name: /Match source/ }));
    await fireEvent.click(await screen.findByText('Go remote'));
    await waitFor(() => expect(feedSource.version).toBe(0));
    expect(feedSource.current?.kind).toBe('profile');
  });

  it('stores one pick at a time, so the UI order is the persisted order', async () => {
    let release = () => {};
    setFeedSource.mockImplementation(() => new Promise<void>((r) => (release = r)));
    const first = feedSource.choose({ source: 'saved_search', saved_search_id: 1 });
    expect(await feedSource.choose({ source: 'profile' })).toBe(false);
    expect(setFeedSource).toHaveBeenCalledTimes(1);
    release();
    expect(await first).toBe(true);
    expect(feedSource.current).toEqual({ kind: 'saved_search', id: 1 });
  });

  it('shows a neutral label until the server has answered', async () => {
    listSavedSearches.mockResolvedValue([]);
    feedSource.current = null;
    render(FeedSourceMenu);
    expect(screen.getByRole('button', { name: 'Match source: Loading…' })).toBeTruthy();
  });

  it('drops a previous account’s source and searches on sign-out', async () => {
    let finish!: (value: ReturnType<typeof search>[]) => void;
    listSavedSearches.mockReturnValueOnce(new Promise((resolve) => (finish = resolve)));
    feedSource.current = { kind: 'saved_search', id: 1 };
    const loading = feedSource.loadSearches();
    resetUserStores();
    finish([search(1, 'Old account search')]);
    await loading;
    expect(feedSource.current).toBeNull();
    expect(feedSource.searches).toEqual([]);
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

  describe('with a host that owns the menu by ref', () => {
    it('opens from the caught-up button, and the click that opened it does not close it', async () => {
      listSavedSearches.mockResolvedValue([search(1, 'Go remote')]);
      const { component } = render(FeedSourceMenu);
      render(FeedCaughtUp, { focus: false, onChooseSource: () => component.openMenu() });
      const choose = screen.getByRole('button', { name: 'Choose another search' });
      choose.focus();
      await fireEvent.click(choose);
      expect(await screen.findByRole('menu')).toBeTruthy();
      await fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' });
      expect(screen.queryByRole('menu')).toBeNull();
      await waitFor(() => expect(document.activeElement).toBe(choose));
    });

    it('hands focus to the host after a pick instead of its own pill', async () => {
      listSavedSearches.mockResolvedValue([search(1, 'Go remote')]);
      const onpicked = vi.fn();
      render(FeedSourceMenu, { onpicked });
      await fireEvent.click(screen.getByRole('button', { name: /Match source/ }));
      await fireEvent.click(await screen.findByText('Go remote'));
      await waitFor(() => expect(onpicked).toHaveBeenCalledTimes(1));
    });

    it('does not pull focus back when the menu is gone before the server answers', async () => {
      listSavedSearches.mockResolvedValue([search(1, 'Go remote')]);
      let release = () => {};
      setFeedSource.mockImplementation(() => new Promise<void>((r) => (release = r)));
      const { unmount } = render(FeedSourceMenu);
      const elsewhere = document.body.appendChild(document.createElement('button'));
      await fireEvent.click(screen.getByRole('button', { name: /Match source/ }));
      await fireEvent.click(await screen.findByText('Go remote'));
      unmount(); // left the feed, or switched mode
      elsewhere.focus();
      release();
      await waitFor(() => expect(feedSource.version).toBe(1));
      expect(document.activeElement).toBe(elsewhere);
    });
  });
});
