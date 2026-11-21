import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';

const meFeed = vi.fn();
vi.mock('#lib/api.js', () => ({ api: { meFeed: (...a: unknown[]) => meFeed(...a), plans: vi.fn() } }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/matches') } }));
vi.mock('#lib/dismissedJobs.svelte.js', () => ({ ensureDismissedLoaded: vi.fn(), isDismissed: () => false }));

const { default: MatchFeedList } = await import('./MatchFeedList.svelte');

describe('MatchFeedList', () => {
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
});
