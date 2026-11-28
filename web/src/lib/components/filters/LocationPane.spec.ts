import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { FilterStore } from '#lib/filters.js';
import { StagedFilters } from '#lib/stagedFilters.svelte.js';
import LocationPane from './LocationPane.svelte';

vi.mock('$app/env', () => ({ browser: false, PUBLIC_OPEN_WITHIN: '' }));
vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/jobs'), state: {} },
}));
vi.mock('$app/navigation', () => ({ afterNavigate() {}, goto: vi.fn(), replaceState: vi.fn() }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => false, currentUser: () => null }));
vi.mock('#lib/profile.svelte.js', () => ({
  profileStore: { profile: null, loaded: true, ensureLoaded() {} },
}));

// The two stores the pane is mounted with. Both keep their setter as a prototype method
// reading a private field, so a detached reference thrown from the checkbox fails here.
describe('country-only checkbox', () => {
  it('writes the live FilterStore', async () => {
    const store = new FilterStore(new URLSearchParams('countries=ch'));
    render(LocationPane, { store });

    await fireEvent.click(screen.getByRole('checkbox', { name: /Only jobs located in one country/ }));

    expect(store.value.countriesOnly).toBe(true);
    expect(store.params.get('countries_only')).toBe('true');
  });

  it('writes the modal StagedFilters', async () => {
    const staged = new StagedFilters();
    staged.seed(new FilterStore(new URLSearchParams('countries=ch')).value);
    render(LocationPane, { store: staged });

    await fireEvent.click(screen.getByRole('checkbox', { name: /Only jobs located in one country/ }));

    expect(staged.value.countriesOnly).toBe(true);
    expect(staged.params().get('countries_only')).toBe('true');
  });
});
