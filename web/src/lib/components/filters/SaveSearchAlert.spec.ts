import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import SaveSearchAlert from './SaveSearchAlert.svelte';

const state = vi.hoisted(() => ({
  items: [] as { id: number; name: string; query: string }[],
  subs: [] as { id: number; saved_search_id: number; channel: string }[],
}));

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
vi.mock('#lib/signin.js', () => ({ promptSignIn: vi.fn() }));
vi.mock('#lib/savedSearches.svelte.js', () => ({
  savedSearches: {
    get items() {
      return state.items;
    },
    ensureLoaded: vi.fn(),
  },
}));
vi.mock('#lib/notifications.svelte.js', () => ({
  notifications: {
    get subscriptions() {
      return state.subs;
    },
    ensureLoaded: vi.fn(),
  },
}));

const notice = 'Add a filter to get alerts: this search covers every job.';

beforeEach(() => {
  state.items = [];
  state.subs = [];
});

describe('SaveSearchAlert', () => {
  it.each(['min_skill_match=70', ''])('offers no alert on a saved query that does not narrow: %j', (query) => {
    state.items = [{ id: 1, name: 'mine', query }];
    render(SaveSearchAlert, { query, alerts: 'manage' });
    expect(screen.getByText(notice)).toBeTruthy();
    expect(screen.queryByText('Get new-job alerts')).toBeNull();
  });

  it('keeps the way to an existing subscription once its search stops narrowing', () => {
    state.items = [{ id: 1, name: 'mine', query: 'min_skill_match=70' }];
    state.subs = [{ id: 9, saved_search_id: 1, channel: 'email' }];
    render(SaveSearchAlert, { query: 'min_skill_match=70', alerts: 'manage' });
    expect(screen.queryByText(notice)).toBeNull();
    expect(screen.getByText('Get new-job alerts')).toBeTruthy();
  });

  it('offers alerts on a saved query that narrows', () => {
    state.items = [{ id: 1, name: 'mine', query: 'seniority=senior' }];
    render(SaveSearchAlert, { query: 'seniority=senior', alerts: 'manage' });
    expect(screen.queryByText(notice)).toBeNull();
    expect(screen.getByText('Get new-job alerts')).toBeTruthy();
  });
});
