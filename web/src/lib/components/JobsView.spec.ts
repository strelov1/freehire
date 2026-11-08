import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SvelteMap } from 'svelte/reactivity';
import { tick } from 'svelte';
import JobsView from './JobsView.svelte';
import SavedSearches from './SavedSearches.svelte';
import { StagedFilters } from '#lib/stagedFilters.svelte.js';
import { emptyFilters } from '#lib/facetModel.js';
import type { Job } from '#lib/types.js';

const session = new SvelteMap<string, number | null>();
vi.mock('$app/env', () => ({ browser: false, PUBLIC_OPEN_WITHIN: '' }));
vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/jobs'), state: {} },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ afterNavigate() {}, goto: vi.fn(), replaceState: vi.fn() }));
vi.mock('#lib/auth.svelte.js', () => ({
  isAuthenticated: () => session.get('id') != null,
  currentUser: () => (session.get('id') == null ? null : { id: session.get('id') }),
}));
vi.mock('#lib/analytics.js', () => ({ track() {} }));
const { searchJobs, saved, profileStore } = vi.hoisted(() => ({
  profileStore: { profile: null as { skills: string[] } | null, loaded: true, ensureLoaded() {} },
  searchJobs: vi.fn(),
  saved: { items: [], ensureLoaded: vi.fn(), update: vi.fn() },
}));
vi.mock('#lib/profile.svelte.js', () => ({ profileStore }));
vi.mock('#lib/savedSearches.svelte.js', () => ({ savedSearches: saved }));
vi.mock('#lib/api.js', () => ({
  ApiError: class extends Error {},
  api: {
    searchJobs,
    facetCounts: async () => ({ total: 0, facets: {} }),
    listViewedSlugs: async () => [],
    listSavedSlugs: async () => [],
    listDismissedSlugs: async () => [],
    listAppliedCompanySlugs: async () => [],
  },
}));
// Keep the list, controls and summary real; the closed modal and onboarding are unrelated.
vi.mock('./filters/FilterModal.svelte', () => ({ default() {} }));
vi.mock('./onboarding/OnboardingBanner.svelte', () => ({ default() {} }));
vi.mock('./onboarding/OnboardingAlertBanner.svelte', () => ({ default() {} }));

beforeEach(() => {
  session.set('id', null);
  profileStore.profile = null;
  searchJobs.mockReset().mockResolvedValue({ items: [], total: 0, hasMore: false });
  saved.ensureLoaded.mockReset().mockResolvedValue(undefined);
  localStorage.clear();
});

function mountList(params: string, scope = {}) {
  return render(JobsView, {
    initial: { items: [], total: 0, hasMore: false },
    initialParams: params,
    currentPage: 1,
    scope,
  });
}

describe('minimum skill match from the URL or a saved search', () => {
  const hint = 'Sort by best match';

  it('offers the match sort, with a removable chip, for a reader with skills', () => {
    session.set('id', 7);
    profileStore.profile = { skills: ['go'] };
    mountList('min_skill_match=70');
    expect(screen.getByRole('button', { name: hint })).toBeTruthy();
    expect(screen.getByText('70%+')).toBeTruthy();
  });

  it('stops offering it once the feed is sorted by match', () => {
    session.set('id', 7);
    profileStore.profile = { skills: ['go'] };
    mountList('min_skill_match=70&sort=match');
    expect(screen.queryByRole('button', { name: hint })).toBeNull();
  });

  const lowJob = (n: number) =>
    ({
      public_slug: `low-${n}`,
      title: `Low match ${n}`,
      company: 'Acme',
      skills: ['go', 'kafka', 'aws', 'spark'],
    }) as unknown as Job;

  it('shows the server page as served under best match, with a lower-bound total', () => {
    session.set('id', 7);
    profileStore.profile = { skills: ['go'] };
    render(JobsView, {
      initial: { items: [lowJob(1)], total: 41, hasMore: true, totalIsLowerBound: true },
      initialParams: 'min_skill_match=70&sort=match',
      currentPage: 1,
      scope: {},
    });
    expect(screen.getByText(/Low match 1/)).toBeTruthy();
    expect(screen.getAllByTitle('At least 41 jobs').length).toBeGreaterThan(0);
  });

  it('still trims the page in the browser under any other ordering', () => {
    session.set('id', 7);
    profileStore.profile = { skills: ['go'] };
    render(JobsView, {
      initial: { items: [lowJob(1)], total: 41, hasMore: true },
      initialParams: 'min_skill_match=70',
      currentPage: 1,
      scope: {},
    });
    expect(screen.queryByText(/Low match 1/)).toBeNull();
  });

  it('is restored by applying a saved set, and reset by clearing', () => {
    const store = new StagedFilters();
    store.apply('seniority=senior&min_skill_match=70');
    expect(store.value.minSkillMatch).toBe(70);
    store.clear();
    expect(store.value.minSkillMatch).toBeNull();
  });

  it('claims nothing for a reader with no skills to compare', () => {
    session.set('id', 7);
    mountList('min_skill_match=70');
    expect(screen.queryByRole('button', { name: hint })).toBeNull();
    expect(screen.queryByText('70%+')).toBeNull();
  });
});

describe('personal job filters', () => {
  it('moves Hide viewed out of the toolbar, with no separate Hiring again button', () => {
    session.set('id', 7);
    mountList('');
    expect(screen.queryByRole('checkbox', { name: 'Hide viewed' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Hiring again: only companies you have worked at' })).toBeNull();
  });

  it.each(['former_employers', 'hide_seen'])('reloads %s on sign-in, account switch and sign-out', async (param) => {
    mountList(`${param}=true`);
    await tick();
    expect(searchJobs).not.toHaveBeenCalled();
    for (const [id, calls] of [
      [7, 1],
      [8, 2],
      [null, 3],
    ] as const) {
      session.set('id', id);
      await waitFor(() => expect(searchJobs).toHaveBeenCalledTimes(calls));
      expect(searchJobs.mock.lastCall?.[0].get(param)).toBe('true');
    }
  });

  it('starts a new reader on page one instead of retaining a deep offset', async () => {
    render(JobsView, {
      initial: { items: [], total: 100, hasMore: false },
      initialParams: 'former_employers=true',
      currentPage: 4,
    });
    await tick();
    session.set('id', 7);
    await waitFor(() => expect(searchJobs).toHaveBeenCalled());
    expect(searchJobs.mock.lastCall?.[2]).toBe(0);
  });

  it('ignores feed-only filters in scoped searches', async () => {
    mountList('hide_seen=true&former_employers=true', { company_slug: 'acme' });
    const sort = screen.getAllByRole('combobox', { name: 'Sort jobs' })[0];
    if (!sort) throw new Error('Expected a sort control');
    await fireEvent.change(sort, { target: { value: 'views' } });
    await waitFor(() => expect(searchJobs.mock.lastCall?.[0].get('sort')).toBe('view_count'));
    for (const [params] of searchJobs.mock.calls as [URLSearchParams][]) {
      expect(params.get('company_slug')).toBe('acme');
      expect(params.has('hide_seen')).toBe(false);
      expect(params.has('former_employers')).toBe(false);
    }
  });

  it('explains why the sidebar cannot save a former-employer search', () => {
    mountList('former_employers=true');
    expect(screen.getByText('Turn off Hiring again to save this search or set up alerts.')).toBeTruthy();
    expect(screen.queryByRole('button', { name: /Save (this search|filter)/ })).toBeNull();
  });

  it('also blocks saving from the modal saved-search tab', () => {
    session.set('id', 7);
    const store = new StagedFilters();
    store.seed({ ...emptyFilters(), formerEmployers: true });
    render(SavedSearches, { store });
    expect(screen.getByText('Turn off Hiring again to save this search or set up alerts.')).toBeTruthy();
    expect(screen.queryByRole('button', { name: /Save (this search|filter)/ })).toBeNull();
  });
});
