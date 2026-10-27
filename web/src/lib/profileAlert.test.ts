import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { UserProfile } from '#lib/types.js';

const profile: UserProfile = {
  specializations: ['backend'],
  skills: ['go'],
  seniorities: [],
  excluded_skills: [],
  excluded_sources: [],
  excluded_companies: [],
  location_preferences: null,
  derived_location: null,
  cv: null,
  created_at: null,
  updated_at: null,
};

const { searches, create, remove, subscribe } = vi.hoisted(() => ({
  searches: { items: [] as { id: number; derived_from_profile: boolean }[] },
  create: vi.fn(),
  remove: vi.fn(),
  subscribe: vi.fn(),
}));

vi.mock('#lib/savedSearches.svelte.js', () => ({
  savedSearches: {
    get items() {
      return searches.items;
    },
    create,
    remove,
  },
}));

vi.mock('#lib/notifications.svelte.js', () => ({
  notifications: { subscribe },
}));

vi.mock('#lib/filters.js', async () => {
  const { filtersFromProfile, filtersToParams } = await import('./facetModel');
  return { filtersFromProfile, filtersToParams };
});

beforeEach(() => {
  searches.items = [];
  create.mockReset().mockResolvedValue({ id: 7, derived_from_profile: true });
  remove.mockReset().mockResolvedValue(undefined);
  subscribe.mockReset().mockResolvedValue(undefined);
});

describe('enableProfileAlert', () => {
  it('creates the profile search and subscribes it on the channel', async () => {
    const { enableProfileAlert } = await import('./profileAlert');
    await enableProfileAlert(profile, 'telegram');
    expect(create).toHaveBeenCalledWith('My profile', expect.stringContaining('category=backend'), true);
    expect(subscribe).toHaveBeenCalledWith(7, 'telegram');
  });

  // The server allows one profile search per user; creating another would conflict
  // rather than add a channel.
  it('reuses an existing profile search', async () => {
    searches.items = [{ id: 42, derived_from_profile: true }];
    const { enableProfileAlert } = await import('./profileAlert');
    await enableProfileAlert(profile, 'email');
    expect(create).not.toHaveBeenCalled();
    expect(subscribe).toHaveBeenCalledWith(42, 'email');
  });

  it('removes a search it created when the subscribe fails, and rethrows', async () => {
    subscribe.mockRejectedValue(new Error('nope'));
    const { enableProfileAlert } = await import('./profileAlert');
    await expect(enableProfileAlert(profile, 'email')).rejects.toThrow('nope');
    expect(remove).toHaveBeenCalledWith(7);
  });

  // An existing search is the user's own; a failed re-subscribe must not delete it.
  it('never removes a search it did not create', async () => {
    searches.items = [{ id: 42, derived_from_profile: true }];
    subscribe.mockRejectedValue(new Error('nope'));
    const { enableProfileAlert } = await import('./profileAlert');
    await expect(enableProfileAlert(profile, 'email')).rejects.toThrow('nope');
    expect(remove).not.toHaveBeenCalled();
  });
});
