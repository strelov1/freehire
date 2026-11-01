import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SvelteMap } from 'svelte/reactivity';
import { FilterStore } from '#lib/filters.js';
import FilterModal from './FilterModal.svelte';

const session = new SvelteMap<string, number | null>();
vi.mock('$app/env', () => ({ browser: false, PUBLIC_OPEN_WITHIN: '' }));
vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/jobs'), state: {} },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ afterNavigate() {}, goto: vi.fn(), replaceState: vi.fn() }));
vi.mock('#lib/auth.svelte.js', () => ({
  isAuthenticated: () => session.get('id') != null,
  currentUser: () => session.get('id') == null ? null : { id: session.get('id') },
}));
vi.mock('#lib/analytics.js', () => ({ track() {} }));
vi.mock('#lib/profile.svelte.js', () => ({
  profileStore: { profile: null, loaded: true, ensureLoaded() {} },
}));
vi.mock('#lib/notifications.svelte.js', () => ({
  notifications: { telegram: { enabled: false }, ensureLoaded() {} },
}));
vi.mock('#lib/api.js', () => ({
  ApiError: class extends Error {},
  api: {
    listCompanies: async () => ({ items: [] }),
    searchCities: async () => [],
  },
}));
vi.mock('./AiFilterButton.svelte', () => ({ default() {} }));

beforeEach(() => {
  session.set('id', 7);
});

describe('Hiring again in the Company filter pane', () => {
  it('stages changes, discards a closed draft and applies through the normal action', async () => {
    const store = new FilterStore();
    const view = render(FilterModal, { store, formerEmployersAvailable: true, open: true, onClose() {} });
    await fireEvent.click(screen.getByRole('button', { name: 'Company' }));
    const checkbox = screen.getByRole('checkbox', { name: 'Hiring again' }) as HTMLInputElement;
    await fireEvent.click(checkbox);
    expect(checkbox.checked).toBe(true);
    expect(store.value.formerEmployers).toBe(false);
    expect(screen.getByRole('button', { name: /Company.*1 selected/ })).toBeTruthy();

    await view.rerender({ open: false });
    await view.rerender({ open: true });
    await waitFor(() => expect((screen.getByRole('checkbox', { name: 'Hiring again' }) as HTMLInputElement).checked).toBe(false));
    await fireEvent.click(screen.getByRole('checkbox', { name: 'Hiring again' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Show jobs' }));
    expect(store.value.formerEmployers).toBe(true);
    expect(store.params.get('former_employers')).toBe('true');
  });

  it.each([
    { signedIn: false, standalone: true },
    { signedIn: true, standalone: false },
  ])('keeps the personal filter unavailable for $signedIn/$standalone', async ({ signedIn, standalone }) => {
    session.set('id', signedIn ? 7 : null);
    render(FilterModal, { store: new FilterStore(), formerEmployersAvailable: standalone, open: true, onClose() {} });
    await fireEvent.click(screen.getByRole('button', { name: 'Company' }));
    expect(screen.queryByRole('checkbox', { name: 'Hiring again' })).toBeNull();
  });

  it('can find and select it through the normal filter search', async () => {
    const store = new FilterStore();
    render(FilterModal, { store, formerEmployersAvailable: true, open: true, onClose() {} });
    await fireEvent.input(screen.getByRole('textbox', { name: 'Search filters' }), { target: { value: 'former employers' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Hiring again' }));
    expect(store.value.formerEmployers).toBe(false);
    await fireEvent.click(screen.getByRole('button', { name: 'Show jobs' }));
    expect(store.value.formerEmployers).toBe(true);
  });
});
