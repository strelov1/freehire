import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { isAuthenticated } from '#lib/auth.svelte.js';
import { FilterStore } from '#lib/filters.js';
import FilterSummary from './FilterSummary.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/jobs'), state: {} },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: vi.fn(), currentUser: () => null }));

beforeEach(() => {
  vi.mocked(isAuthenticated).mockReturnValue(false);
});

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

describe('FilterSummary personal filters', () => {
  const query = 'former_employers=true&watchlist=true&hide_seen=true';
  const chips = ['Your former employers', 'Your watchlist', 'Hidden'];

  it('hides personal filter chips for signed-out visitors', () => {
    renderSummary(query);
    for (const chip of chips) {
      expect(screen.queryByText(chip)).toBeNull();
    }
  });

  it('shows personal filter chips for signed-in visitors', () => {
    vi.mocked(isAuthenticated).mockReturnValue(true);
    renderSummary(query);
    for (const chip of chips) {
      expect(screen.getByText(chip)).toBeTruthy();
    }
  });
});
