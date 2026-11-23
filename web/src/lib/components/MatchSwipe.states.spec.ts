import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';

const meFeed = vi.fn();
const saveJob = vi.fn();
vi.mock('#lib/api.js', () => ({
  api: {
    meFeed: (...a: unknown[]) => meFeed(...a),
    saveJob: (...a: unknown[]) => saveJob(...a),
    listSavedSearches: vi.fn().mockResolvedValue([]),
  },
}));
vi.mock('#lib/analytics.js', () => ({ track: vi.fn() }));
vi.mock('#lib/announce.svelte.js', () => ({ announce: vi.fn() }));
vi.mock('$app/paths', () => ({
  resolve: (path: string, p?: Record<string, string>) => path.replace(/\[(\w+)\]/g, (_, k) => p?.[k] ?? ''),
}));
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/matches') } }));

const goto = vi.fn();
// A sign-in navigation that has not finished; released after each test so the next one starts clean.
let finishNavigation = () => {};
const pendingNavigation = () => new Promise<void>((resolve) => (finishNavigation = resolve));
vi.mock('$app/navigation', () => ({ goto: (...a: unknown[]) => goto(...a) }));

const { default: MatchSwipe } = await import('./MatchSwipe.svelte');
const { feedSession } = await import('#lib/feedSession.svelte.js');

describe('MatchSwipe states', () => {
  beforeEach(() => {
    feedSession.reset();
    goto.mockReset();
    meFeed.mockReset();
    saveJob.mockReset();
  });
  afterEach(async () => {
    finishNavigation();
    await Promise.resolve();
  });

  it('sends an ended session (401) back through sign-in, not to an error', async () => {
    goto.mockReturnValue(pendingNavigation());
    meFeed.mockRejectedValue(new ApiError(401, 'unauthorized'));
    render(MatchSwipe, { onClose: () => {} });
    expect(await screen.findByText('Your session ended')).toBeTruthy();
    expect(goto).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("You're all caught up")).toBeNull();
    expect(screen.queryByText('Failed to load your matches.')).toBeNull();
  });

  it('a save refused with 401 signs in again: no failure toast, no put-back', async () => {
    goto.mockReturnValue(pendingNavigation());
    meFeed.mockResolvedValue({
      data: [
        {
          is_new: false,
          public_slug: 'a',
          title: 'A',
          company: 'Acme',
          skills: [],
          match: { why: [], may_not_fit: [], unknown: [] },
        },
      ],
      meta: { source: 'profile', total: 1 },
    });
    saveJob.mockRejectedValue(new ApiError(401, 'unauthorized'));
    render(MatchSwipe, { onClose: () => {} });
    await fireEvent.click(await screen.findByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Your session ended')).toBeTruthy();
    expect(screen.queryByText("Didn't save — try again")).toBeNull();
    expect(goto).toHaveBeenCalledTimes(1);
  });

  it('shows the empty-profile screen and the date-sort banner', async () => {
    meFeed.mockResolvedValueOnce({ data: [], meta: { source: 'profile', total: 0, reason: 'no_profile' } });
    const { unmount } = render(MatchSwipe, { onClose: () => {} });
    expect(await screen.findByText('Tell us about yourself — upload a CV or fill in your profile')).toBeTruthy();
    unmount();
    meFeed.mockResolvedValueOnce({ data: [], meta: { source: 'profile', total: 0, degraded: 'no_match_vector' } });
    render(MatchSwipe, { onClose: () => {} });
    expect(await screen.findByText('add skills')).toBeTruthy();
    expect(await screen.findByText("You're all caught up")).toBeTruthy();
  });
});
