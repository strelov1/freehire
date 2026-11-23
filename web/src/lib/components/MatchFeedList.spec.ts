import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';

const meFeed = vi.fn();
const markFeedSeen = vi.fn();
const listSavedSearches = vi.fn();
const setFeedSource = vi.fn();
vi.mock('#lib/api.js', () => ({
  api: {
    meFeed: (...a: unknown[]) => meFeed(...a),
    markFeedSeen: (...a: unknown[]) => markFeedSeen(...a),
    listSavedSearches: (...a: unknown[]) => listSavedSearches(...a),
    setFeedSource: (...a: unknown[]) => setFeedSource(...a),
    plans: vi.fn(),
  },
}));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/matches') } }));
const dismissed = new Set<string>();
vi.mock('#lib/dismissedJobs.svelte.js', () => ({
  ensureDismissedLoaded: vi.fn(),
  isDismissed: (slug: string) => dismissed.has(slug),
}));

// The sentinel after the last new card: reveal() scrolls it into view.
let reveal = () => {};
vi.stubGlobal(
  'IntersectionObserver',
  class {
    constructor(private cb: (e: { isIntersecting: boolean }[]) => void) {}
    observe() {
      reveal = () => this.cb([{ isIntersecting: true }]);
    }
    disconnect() {}
  },
);

const { default: MatchFeedList } = await import('./MatchFeedList.svelte');
const { feedSource } = await import('#lib/feedSource.svelte.js');

const card = (slug: string, is_new = false) => ({
  is_new,
  public_slug: slug,
  title: slug,
  company: 'Acme',
  skills: [] as string[],
  match: { why: [], may_not_fit: [], unknown: [] },
});
const page = (slugs: string[], meta: Record<string, unknown> = {}, fresh: string[] = []) => ({
  data: slugs.map((s) => card(s, fresh.includes(s))),
  meta: { source: 'profile', total: 99, ...meta },
});

describe('MatchFeedList', () => {
  beforeEach(() => {
    meFeed.mockReset();
    markFeedSeen.mockReset();
    markFeedSeen.mockResolvedValue(undefined);
    dismissed.clear();
    reveal = () => {};
  });

  it('keeps "Show more" when every loaded card is hidden but another page exists', async () => {
    dismissed.add('a');
    meFeed.mockResolvedValueOnce(page(['a'], { next_cursor: 'c1' }));
    render(MatchFeedList);
    expect(await screen.findByText('Show more matches')).toBeTruthy();
    expect(screen.queryByText("You're all caught up")).toBeNull();
  });

  it('shows the upgrade screen, not an error, when the plan lapsed (402)', async () => {
    meFeed.mockRejectedValue(new ApiError(402, 'The match feed needs Pro or Ultra'));
    render(MatchFeedList);
    expect(await screen.findByText('Part of Pro and Ultra')).toBeTruthy();
    expect(screen.queryByText('Failed to load your matches.')).toBeNull();
  });

  it('shows the error state for any other failure', async () => {
    meFeed.mockRejectedValue(new ApiError(500, 'boom'));
    render(MatchFeedList);
    expect(await screen.findByText('Failed to load your matches.')).toBeTruthy();
  });

  it('pages with the signed cursor and hides "Show more" on the last page', async () => {
    meFeed.mockResolvedValueOnce(page(['a'], { next_cursor: 'c1' })).mockResolvedValueOnce(page(['b']));
    render(MatchFeedList);
    await fireEvent.click(await screen.findByText('Show more matches'));
    expect(await screen.findByText('b')).toBeTruthy();
    expect(meFeed).toHaveBeenLastCalledWith(20, 'c1');
    expect(screen.queryByText('Show more matches')).toBeNull();
  });

  it('drops cards from a saved search whose filters changed during paging', async () => {
    const search = { source: 'saved_search', saved_search_id: 5 };
    meFeed
      .mockResolvedValueOnce(page(['old'], { ...search, next_cursor: 'c1' }))
      .mockResolvedValueOnce(page([], { ...search, reason: 'source_changed' }))
      .mockResolvedValueOnce(page(['current'], search));
    render(MatchFeedList);
    expect(await screen.findByText('old')).toBeTruthy();
    await fireEvent.click(await screen.findByText('Show more matches'));
    expect(await screen.findByText('current')).toBeTruthy();
    expect(screen.queryByText('old')).toBeNull();
  });

  it.each([
    { on: 'source_changed', next: () => meFeed.mockResolvedValueOnce(page([], { reason: 'source_changed' })) },
    { on: 'a rejected cursor (400)', next: () => meFeed.mockRejectedValueOnce(new ApiError(400, 'invalid cursor')) },
  ])('restarts from page 1 silently on $on', async ({ next }) => {
    meFeed.mockResolvedValueOnce(page(['a'], { next_cursor: 'c1' }));
    next();
    meFeed.mockResolvedValueOnce(page(['a', 'c'], { next_cursor: 'c2' }));
    render(MatchFeedList);
    await fireEvent.click(await screen.findByText('Show more matches'));
    expect(await screen.findByText('c')).toBeTruthy();
    expect(meFeed).toHaveBeenLastCalledWith(20, undefined);
    expect(screen.getAllByText('a')).toHaveLength(1);
    expect(screen.queryByText('Fill in my profile')).toBeNull();
    expect(await screen.findByText('Show more matches')).toBeTruthy();
  });

  it('badges new cards, pauses before the rest, and posts seen once', async () => {
    meFeed.mockResolvedValueOnce(page(['n1', 'r1'], { seen_cursor: 's1', next_cursor: 'c1' }, ['n1']));
    render(MatchFeedList);
    expect(await screen.findByText("That's everything new")).toBeTruthy();
    expect(screen.getAllByText('New')).toHaveLength(1);
    expect(screen.queryByText('r1')).toBeNull();
    // Focus would scroll the list past the new cards above the pause screen.
    expect(document.activeElement).not.toBe(screen.getByText("That's everything new"));
    // Rendering the list is not passing the new cards: seen waits for the scroll.
    expect(markFeedSeen).not.toHaveBeenCalled();
    reveal();
    expect(markFeedSeen).toHaveBeenCalledTimes(1);
    expect(markFeedSeen).toHaveBeenCalledWith('s1');
    await fireEvent.click(screen.getByText('Show the rest'));
    expect(await screen.findByText('r1')).toBeTruthy();
    expect(screen.queryByText("That's everything new")).toBeNull();
    expect(markFeedSeen).toHaveBeenCalledTimes(1);
  });

  it('posts seen when Show the rest is pressed before the scroll registers', async () => {
    meFeed.mockResolvedValueOnce(page(['n1', 'r1'], { seen_cursor: 's1' }, ['n1']));
    render(MatchFeedList);
    await fireEvent.click(await screen.findByText('Show the rest'));
    expect(markFeedSeen).toHaveBeenCalledWith('s1');
    reveal();
    expect(markFeedSeen).toHaveBeenCalledTimes(1);
  });

  it('does not post seen or pause while the new phase continues on the next page', async () => {
    meFeed.mockResolvedValueOnce(page(['n1'], { next_cursor: 'c1' }, ['n1']));
    render(MatchFeedList);
    expect(await screen.findByText('n1')).toBeTruthy();
    expect(screen.queryByText("That's everything new")).toBeNull();
    expect(screen.getByText('Show more matches')).toBeTruthy();
    expect(markFeedSeen).not.toHaveBeenCalled();
  });

  it('shows no badge and no pause when nothing is new', async () => {
    meFeed.mockResolvedValueOnce(page(['r1', 'r2']));
    render(MatchFeedList);
    expect(await screen.findByText('r1')).toBeTruthy();
    expect(screen.queryByText('New')).toBeNull();
    expect(screen.queryByText("That's everything new")).toBeNull();
    expect(markFeedSeen).not.toHaveBeenCalled();
  });

  it('drops a stale New badge when a restart returns the card as seen', async () => {
    meFeed
      .mockResolvedValueOnce(page(['n1', 'r1'], { seen_cursor: 's1', next_cursor: 'c1' }, ['n1']))
      .mockRejectedValueOnce(new ApiError(400, 'invalid cursor'))
      .mockResolvedValueOnce(page(['n1', 'r1', 'r2']));
    render(MatchFeedList);
    await fireEvent.click(await screen.findByText('Show the rest'));
    await fireEvent.click(await screen.findByText('Show more matches'));
    expect(await screen.findByText('r2')).toBeTruthy();
    expect(screen.queryByText('New')).toBeNull();
    expect(screen.queryByText("That's everything new")).toBeNull();
  });

  it('posts seen at the end when new cards have no rest behind them', async () => {
    meFeed.mockResolvedValueOnce(page(['n1'], { seen_cursor: 's1' }, ['n1']));
    render(MatchFeedList);
    expect(await screen.findByText('n1')).toBeTruthy();
    expect(screen.queryByText("That's everything new")).toBeNull();
    expect(markFeedSeen).not.toHaveBeenCalled();
    reveal();
    expect(markFeedSeen).toHaveBeenCalledWith('s1');
  });

  it('ignores a response that lands after the source was switched', async () => {
    const pending = Promise.withResolvers<ReturnType<typeof page>>();
    meFeed.mockReturnValueOnce(pending.promise);
    feedSource.current = { kind: 'saved_search', id: 5 };
    render(MatchFeedList);
    feedSource.version++; // a pick restarted the feed while this request was in flight
    pending.resolve(page(['old'], { source: 'profile' }));
    await Promise.resolve();
    await Promise.resolve();
    expect(feedSource.current).toEqual({ kind: 'saved_search', id: 5 });
  });

  describe('saved-search source', () => {
    beforeEach(() => {
      listSavedSearches.mockResolvedValue([]);
      setFeedSource.mockResolvedValue(undefined);
    });

    it('shows the deleted screen and goes back to the profile', async () => {
      meFeed.mockResolvedValueOnce({
        data: [],
        meta: { source: 'saved_search', saved_search_id: 2, total: 0, reason: 'search_deleted' },
      });
      render(MatchFeedList);
      expect(await screen.findByText('This search was deleted')).toBeTruthy();
      await fireEvent.click(screen.getByText('Back to matching by profile'));
      expect(setFeedSource).toHaveBeenCalledWith({ source: 'profile' });
    });

    it('shows no-matches for an empty first page, never "all caught up"', async () => {
      meFeed.mockResolvedValueOnce({ data: [], meta: { source: 'saved_search', saved_search_id: 2, total: 0 } });
      render(MatchFeedList);
      expect(await screen.findByText('No matches for this search')).toBeTruthy();
      expect(screen.getByText("We don't widen your search to fill the feed.")).toBeTruthy();
      expect(screen.getByText('Edit search').getAttribute('href')).toBe('/my/notifications/searches');
      expect(screen.queryByText("You're all caught up")).toBeNull();
    });

    it('keeps focus on the swipe controls when the background list loads its empty state', async () => {
      const swipeControl = document.createElement('button');
      document.body.append(swipeControl);
      swipeControl.focus();
      try {
        meFeed.mockResolvedValueOnce(page([], { source: 'saved_search', saved_search_id: 2 }));
        render(MatchFeedList, { props: { active: false } });
        await screen.findByRole('heading', { name: 'No matches for this search' });
        expect(document.activeElement).toBe(swipeControl);
      } finally {
        swipeControl.remove();
      }
    });

    it('says an unusable search cannot be applied, not that it has no matches', async () => {
      meFeed.mockResolvedValueOnce({
        data: [],
        meta: {
          source: 'saved_search',
          saved_search_id: 2,
          total: 0,
          reason: 'ignored_filters',
          ignored: [{ param: 'min_skill_match' }],
        },
      });
      render(MatchFeedList);
      expect(await screen.findByText("This search can't be used here")).toBeTruthy();
      expect(screen.queryByText('No matches for this search')).toBeNull();
      expect(screen.getByText('Edit search')).toBeTruthy();
    });

    it('shows the failure and lets the user retry when going back to the profile fails', async () => {
      meFeed.mockResolvedValueOnce({
        data: [],
        meta: { source: 'saved_search', saved_search_id: 2, total: 0, reason: 'search_deleted' },
      });
      setFeedSource.mockClear();
      setFeedSource.mockRejectedValueOnce(new Error('500'));
      render(MatchFeedList);
      await fireEvent.click(await screen.findByText('Back to matching by profile'));
      expect(await screen.findByText("Didn't switch — try again.")).toBeTruthy();
      await fireEvent.click(screen.getByText('Back to matching by profile'));
      expect(setFeedSource).toHaveBeenCalledTimes(2);
    });

    it('keeps the profile empty-state for a profile reason', async () => {
      meFeed.mockResolvedValueOnce({ data: [], meta: { source: 'profile', total: 0, reason: 'no_profile' } });
      render(MatchFeedList);
      expect(await screen.findByText('Tell us what you are looking for')).toBeTruthy();
    });
  });

  it('drops the old cards when another tab switched the source', async () => {
    meFeed.mockResolvedValueOnce(page(['a'], { next_cursor: 'c1' }));
    meFeed.mockRejectedValueOnce(new ApiError(400, 'invalid cursor'));
    meFeed.mockResolvedValueOnce(page(['s'], { source: 'saved_search', saved_search_id: 2 }));
    render(MatchFeedList);
    await fireEvent.click(await screen.findByText('Show more matches'));
    expect(await screen.findByText('s')).toBeTruthy();
    expect(screen.queryByText('a')).toBeNull();
  });
});
