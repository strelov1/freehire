import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { FilterStore } from '#lib/filters.js';
import FilterSummary from './FilterSummary.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/jobs'), state: {} },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => false, currentUser: () => null }));

function renderSummary(params: string) {
  return render(FilterSummary, {
    props: { store: new FilterStore(new URLSearchParams(params)), onOpen: vi.fn(), canSave: true },
  });
}

describe('FilterSummary save control', () => {
  it('offers no save on the bare feed, where there is nothing to save', () => {
    renderSummary('');
    expect(screen.queryByRole('button', { name: /save this search/i })).toBeNull();
  });

  // A minimum skill match narrows the list even though alerts cannot use it on its own,
  // so the panel must still offer to save it.
  it('offers save when the only filter is a minimum skill match', () => {
    renderSummary('min_skill_match=60');
    expect(screen.getByRole('button', { name: /save this search/i })).toBeTruthy();
  });
});
