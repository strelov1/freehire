import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import AlertChannels from './AlertChannels.svelte';

// The store is a runes singleton; a plain object with the same reads stands in
// for it, so each case sets exactly the state it is about.
const state = vi.hoisted(() => ({
  hasPushDevice: false,
  subs: [] as { id: number; saved_search_id: number; channel: string }[],
}));

vi.mock('#lib/notifications.svelte.js', () => ({
  notifications: {
    get telegram() {
      return { enabled: false, linked: false };
    },
    get webhook() {
      return null;
    },
    get hasPushDevice() {
      return state.hasPushDevice;
    },
    forSavedSearch: (id: number, channel: string) =>
      state.subs.find((s) => s.saved_search_id === id && s.channel === channel),
    subscribe: vi.fn(),
    unsubscribe: vi.fn(),
  },
}));

beforeEach(() => {
  state.hasPushDevice = false;
  state.subs = [];
});

describe('AlertChannels — push', () => {
  it('hides Push until the mobile app has registered a device', () => {
    render(AlertChannels, { savedSearchId: 1 });
    expect(screen.queryByText('Push')).toBeNull();
    expect(screen.getByText('Email')).toBeTruthy();
  });

  it('offers Push once a device is registered', () => {
    state.hasPushDevice = true;
    render(AlertChannels, { savedSearchId: 1 });
    expect(screen.getByText('Push')).toBeTruthy();
  });

  it('keeps an existing push subscription reachable so it can be turned off', () => {
    state.subs = [{ id: 9, saved_search_id: 1, channel: 'push' }];
    render(AlertChannels, { savedSearchId: 1 });
    expect(screen.getByText('Push').closest('button')?.getAttribute('aria-pressed')).toBe('true');
  });
});
