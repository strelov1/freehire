import { describe, expect, it, vi } from 'vitest';
import type { UserProfile } from '$lib/types';

/** A promise the test settles by hand, so a second call can be issued while the
 *  first is still in flight. */
function deferred<T = void>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

const staleProfile = { skills: ['go'] } as UserProfile;
const freshProfile = { skills: ['rust'] } as UserProfile;

const { profileStoreMock, ensureLoaded, update } = vi.hoisted(() => ({
  profileStoreMock: { profile: null as UserProfile | null },
  ensureLoaded: vi.fn().mockResolvedValue(undefined),
  update: vi.fn(),
}));

vi.mock('$lib/profile.svelte', () => ({
  profileStore: profileStoreMock,
}));

vi.mock('$lib/savedSearches.svelte', () => ({
  savedSearches: {
    ensureLoaded,
    items: [{ id: 42, derived_from_profile: true }],
    update,
  },
}));

// Not the behavior under test — a stable, distinguishable query per call is enough
// to tell which call's write landed.
vi.mock('$lib/filters', () => ({
  filtersFromProfile: (p: UserProfile) => p,
  filtersToParams: (p: UserProfile) => ({ toString: () => `skills=${p.skills.join(',')}` }),
}));

describe('syncProfileAlert', () => {
  it('does not start a second sync while the first is still in flight, and the second reads the profile as it stands when its turn comes', async () => {
    const { syncProfileAlert } = await import('./profileAlertSync');
    profileStoreMock.profile = staleProfile;
    const first = deferred();
    update.mockReset();
    update.mockImplementationOnce(() => first.promise).mockImplementationOnce(() => Promise.resolve());

    // Both calls are issued while the profile still reads `staleProfile` — a
    // regression that captured `profileStore.profile` at CALL time (instead of at
    // the queued job's own execution time) would bake `staleProfile`'s query into
    // the second call right here, before the mutation below ever has a chance to
    // matter.
    const a = syncProfileAlert();
    const b = syncProfileAlert();

    await flush();
    expect(update).toHaveBeenCalledTimes(1);

    // The profile changes while the second call is still queued, waiting on the
    // first. The queue's contract (per serialQueue.test.ts: "lets a job read state
    // its predecessor wrote") is that the second call's own execution — not its
    // enqueue — reads whatever is current by the time it actually runs.
    profileStoreMock.profile = freshProfile;
    first.resolve(undefined);
    await a;
    await b;

    expect(update).toHaveBeenCalledTimes(2);
    expect(update.mock.calls[0]?.[1]).toEqual({ query: 'skills=go' });
    expect(update.mock.calls[1]?.[1]).toEqual({ query: 'skills=rust' });
  });

  it('does nothing when the profile has not loaded', async () => {
    const { syncProfileAlert } = await import('./profileAlertSync');
    profileStoreMock.profile = null;
    update.mockClear();
    ensureLoaded.mockClear();

    await syncProfileAlert();

    expect(ensureLoaded).not.toHaveBeenCalled();
    expect(update).not.toHaveBeenCalled();
  });
});
