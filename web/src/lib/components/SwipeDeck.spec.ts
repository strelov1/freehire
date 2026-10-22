import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api.js';
import type { Job } from '#lib/types.js';
import SwipeDeck from './SwipeDeck.svelte';

// SwipeDeck pulls in SvelteKit runtime modules this test environment doesn't provide
// (no sveltekit() plugin in the `components` vitest project - see vitest.config.ts).
vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/jobs') } }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

const { swipeDeck, dismissJob, saveJob, unsaveJob, undismissJob, facetCounts, recordJobView } =
  vi.hoisted(() => ({
    swipeDeck: vi.fn(),
    dismissJob: vi.fn(),
    saveJob: vi.fn(),
    unsaveJob: vi.fn(),
    undismissJob: vi.fn(),
    facetCounts: vi.fn(),
    recordJobView: vi.fn(),
  }));

// The real ApiError is kept on purpose: the deck tells a job that is gone (404)
// from a transport failure by the class and the status, so a stand-in error
// would not prove what the deck reads.
vi.mock('#lib/api.js', async () => {
  const actual = await vi.importActual<typeof import('#lib/api.js')>('#lib/api.js');
  return {
    ...actual,
    api: {
      ...actual.api,
      swipeDeck,
      dismissJob,
      saveJob,
      unsaveJob,
      undismissJob,
      facetCounts,
      recordJobView,
    },
  };
});

vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
vi.mock('#lib/viewedJobs.svelte.js', () => ({ markViewed: vi.fn() }));

// The filter store mirrors itself into the URL through SvelteKit's navigation
// hooks, which this environment does not provide. The deck reads the filters
// as a query string, and nothing else about them is under test.
vi.mock('#lib/filters.js', () => ({
  FilterStore: class {
    applied = new URLSearchParams();
    dispose() {}
  },
  filtersToParams: () => new URLSearchParams(),
}));

// Heavy children whose own mount effects are not under test: the match block
// fetches per card, and the filter drawer is a modal the deck never opens.
vi.mock('./JobMatch.svelte', async () => {
  const stub = await import('../../../vitest-stubs/EmptyComponent.svelte');
  return { default: stub.default };
});
vi.mock('./filters/FilterModal.svelte', async () => {
  const stub = await import('../../../vitest-stubs/EmptyComponent.svelte');
  return { default: stub.default };
});
vi.mock('./States.svelte', async () => {
  const stub = await import('../../../vitest-stubs/EmptyComponent.svelte');
  return { default: stub.default };
});

const gone = {
  public_slug: 'gone-job',
  title: 'Gone Job',
  company: 'Acme',
  skills: [] as string[],
} as Job;
const next = {
  public_slug: 'next-job',
  title: 'Next Job',
  company: 'Globex',
  skills: [] as string[],
} as Job;

const goneCard = 'Job: Gone Job at Acme';
const nextCard = 'Job: Next Job at Globex';

const undoButton = () => screen.getByLabelText('Undo last') as HTMLButtonElement;

// The judged card flies off for FLY_MS before it is dropped from the queue.
const afterFlyOff = () => new Promise((resolve) => setTimeout(resolve, 400));

beforeEach(() => {
  swipeDeck.mockReset().mockResolvedValue({ items: [gone, next] });
  dismissJob.mockReset().mockResolvedValue(undefined);
  saveJob.mockReset().mockResolvedValue(undefined);
  unsaveJob.mockReset().mockResolvedValue(undefined);
  undismissJob.mockReset().mockResolvedValue(undefined);
  facetCounts.mockReset().mockResolvedValue({ total: 0, facets: {}, stats: {} });
  recordJobView.mockReset().mockResolvedValue(undefined);
});

describe('the swipe deck', () => {
  // A job that was renamed upstream 404s on save and on dismiss. Restoring the
  // card left the user on a card they could never get past: the card went back
  // on top of the queue, and the fly-off that followed dropped the copy
  // underneath it instead.
  it('leaves a card alone when the job is gone', async () => {
    dismissJob.mockRejectedValue(new ApiError(404, 'not found'));
    render(SwipeDeck, { props: {} });

    await vi.waitFor(() => expect(screen.getByLabelText(goneCard)).toBeTruthy());

    await fireEvent.click(screen.getByLabelText('Skip (dismiss)'));

    // The gone card is dropped and the deck moves on.
    await vi.waitFor(() => expect(screen.queryByLabelText(goneCard)).toBeNull());
    expect(screen.getByLabelText(nextCard)).toBeTruthy();
    // Undo must not bring a job that no longer exists back.
    expect(undoButton().disabled).toBe(true);
  });

  // Every other failure is transient enough to retry, so the card comes back -
  // that is the behavior this fix must not take away.
  it('restores the card when the request fails for another reason', async () => {
    dismissJob.mockRejectedValue(new ApiError(500, 'server error'));
    render(SwipeDeck, { props: {} });

    await vi.waitFor(() => expect(screen.getByLabelText(goneCard)).toBeTruthy());

    await fireEvent.click(screen.getByLabelText('Skip (dismiss)'));
    await afterFlyOff();

    // The card is back on top, so the deck never reaches the next one.
    expect(screen.getByLabelText(goneCard)).toBeTruthy();
    expect(screen.queryByLabelText(nextCard)).toBeNull();
  });
});
