import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { FacetCounts } from '#lib/types.js';
import type { CityTable } from '#lib/worldMap.js';
import { FilterStore } from '#lib/filters.js';
import JobsMapDialog from './JobsMapDialog.svelte';

vi.mock('$app/env', () => ({ browser: false }));
const { page } = vi.hoisted(() => ({ page: { data: {} as { locale?: string }, url: new URL('http://localhost/jobs') } }));
vi.mock('$app/state', () => ({ page }));
vi.mock('$app/navigation', () => ({ replaceState: vi.fn(), goto: vi.fn() }));

const { facetCounts, loadCityTable } = vi.hoisted(() => ({
  facetCounts: vi.fn<(params: URLSearchParams, opts?: { facets?: string[] }) => Promise<FacetCounts>>(),
  loadCityTable: vi.fn<() => Promise<CityTable | null>>(),
}));

vi.mock('#lib/api.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('#lib/api.js')>();
  return { ...actual, api: { ...actual.api, facetCounts } };
});
vi.mock('#lib/geoCities.js', () => ({ loadCityTable }));

const TABLE: CityTable = {
  br: { 'sao paulo': [-23.56, -46.63], campinas: [-22.9, -47.1] },
  mx: { 'mexico city': [19.43, -99.13] },
};

function counts(total: number, facets: FacetCounts['facets']): FacetCounts {
  return { total, facets, stats: {}, companies_total: 0, top_companies: [] };
}

const WORLD = counts(1_000, { countries: { br: 700, mx: 250, us: 50 }, cities: {}, regions: { global: 40 } });
const BRAZIL = counts(700, {
  countries: { br: 700, mx: 3 },
  cities: { 'São Paulo': 500, Campinas: 60, 'Mexico City': 3, Brazil: 9 },
  regions: { global: 5 },
});

const SAO_PAULO = counts(500, { countries: { br: 500 }, cities: { 'São Paulo': 500, Campinas: 2 }, regions: { global: 2 } });

// A store seeded through its own writes, the way a visitor's clicks would leave it.
function storeWith(...writes: [string, string][]) {
  const store = new FilterStore();
  for (const [param, v] of writes) store.add(param, v);
  return store;
}

const include = (store: FilterStore, param: string) => store.value.facets[param]?.include ?? [];

beforeEach(() => {
  page.data = {};
  facetCounts.mockReset().mockImplementation(async (params) => {
    if (params.get('cities') === 'São Paulo') return SAO_PAULO;
    return params.get('countries') === 'br' ? BRAZIL : WORLD;
  });
  loadCityTable.mockReset().mockResolvedValue(TABLE);
});

describe('JobsMapDialog — what it draws', () => {
  it('draws a bubble per country with jobs and lists the busiest', async () => {
    render(JobsMapDialog, { open: true, store: storeWith() });
    expect(await screen.findByRole('button', { name: 'Brazil: 700 jobs' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Mexico: 250 jobs' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Brazil · 700' })).toBeTruthy();
    const [params, opts] = facetCounts.mock.calls[0] ?? [];
    expect(params?.toString()).toBe('');
    expect(opts?.facets).toEqual(['countries', 'cities', 'regions']);
    expect(loadCityTable).not.toHaveBeenCalled();
  });

  it('draws one selected country as its cities, omitting the ones it cannot place there', async () => {
    render(JobsMapDialog, { open: true, store: storeWith(['countries', 'br']) });
    expect(await screen.findByRole('button', { name: 'São Paulo: 500 jobs' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Campinas: 60 jobs' })).toBeTruthy();
    expect(screen.queryByRole('button', { name: /Mexico City/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /^Brazil/ })).toBeNull();
    expect(screen.getByText('560 of 700 placed in a city')).toBeTruthy();
  });

  it('falls back to the country when the city table cannot be loaded', async () => {
    loadCityTable.mockResolvedValue(null);
    render(JobsMapDialog, { open: true, store: storeWith(['countries', 'br']) });
    expect(await screen.findByRole('button', { name: 'Brazil: 700 jobs' })).toBeTruthy();
    expect(screen.getByText("Couldn't load cities, so the map shows the country.")).toBeTruthy();
  });

  it('fetches nothing while closed', () => {
    render(JobsMapDialog, { open: false, store: storeWith() });
    expect(facetCounts).not.toHaveBeenCalled();
  });
});

describe('JobsMapDialog — clicks narrow the filters', () => {
  it('a country click replaces the selection', async () => {
    const store = storeWith(['countries', 'br'], ['countries', 'mx']);
    render(JobsMapDialog, { open: true, store });
    await fireEvent.click(await screen.findByRole('button', { name: 'Mexico: 250 jobs' }));
    expect(include(store, 'countries')).toEqual(['mx']);
  });

  it('a click on the selected country deselects it', async () => {
    loadCityTable.mockResolvedValue(null);
    const store = storeWith(['countries', 'br']);
    render(JobsMapDialog, { open: true, store });
    await fireEvent.click(await screen.findByRole('button', { name: 'Brazil: 700 jobs' }));
    expect(include(store, 'countries')).toEqual([]);
  });

  // Location facets OR together on the server: countries=br&cities=X is all of Brazil.
  // A city click must therefore drop the country, or it would narrow nothing.
  it('a city click selects that city in place of its country', async () => {
    const store = storeWith(['countries', 'br']);
    render(JobsMapDialog, { open: true, store });
    await fireEvent.click(await screen.findByRole('button', { name: 'São Paulo: 500 jobs' }));
    expect(include(store, 'cities')).toEqual(['São Paulo']);
    expect(include(store, 'countries')).toEqual([]);
  });

  it("keeps drawing the country's cities once a city is selected", async () => {
    const store = storeWith(['countries', 'br']);
    render(JobsMapDialog, { open: true, store });
    await fireEvent.click(await screen.findByRole('button', { name: 'São Paulo: 500 jobs' }));
    expect(await screen.findByRole('button', { name: 'Show 500 jobs' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Campinas: 60 jobs' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'World view' })).toBeTruthy();
  });

  it('a second city click replaces the first, since cities OR together', async () => {
    const store = storeWith(['countries', 'br']);
    render(JobsMapDialog, { open: true, store });
    await fireEvent.click(await screen.findByRole('button', { name: 'São Paulo: 500 jobs' }));
    await fireEvent.click(await screen.findByRole('button', { name: 'Campinas: 60 jobs' }));
    expect(include(store, 'cities')).toEqual(['Campinas']);
  });

  it('a country click turns Remote worldwide off, so the list switches rather than widens', async () => {
    const store = storeWith(['regions', 'global']);
    render(JobsMapDialog, { open: true, store });
    await fireEvent.click(await screen.findByRole('button', { name: 'Brazil: 700 jobs' }));
    expect(include(store, 'countries')).toEqual(['br']);
    expect(include(store, 'regions')).toEqual([]);
  });

  it('World view clears the country and city filters together', async () => {
    const store = storeWith(['countries', 'br'], ['cities', 'São Paulo']);
    render(JobsMapDialog, { open: true, store });
    await fireEvent.click(await screen.findByRole('button', { name: 'World view' }));
    expect(include(store, 'countries')).toEqual([]);
    expect(include(store, 'cities')).toEqual([]);
  });

  it('Remote worldwide replaces a place when turned on', async () => {
    const store = storeWith(['countries', 'br'], ['cities', 'São Paulo']);
    render(JobsMapDialog, { open: true, store });
    const toggle = await screen.findByRole('button', { name: /Remote worldwide/ });
    expect(toggle.getAttribute('aria-pressed')).toBe('false');
    await fireEvent.click(toggle);
    expect(include(store, 'regions')).toEqual(['global']);
    expect(include(store, 'countries')).toEqual([]);
    expect(include(store, 'cities')).toEqual([]);
  });

  it('Remote worldwide turns off again', async () => {
    const store = storeWith(['regions', 'global']);
    render(JobsMapDialog, { open: true, store });
    const toggle = await screen.findByRole('button', { name: /Remote worldwide/ });
    expect(toggle.getAttribute('aria-pressed')).toBe('true');
    await fireEvent.click(toggle);
    expect(include(store, 'regions')).toEqual([]);
  });
});

describe('JobsMapDialog — Show N jobs', () => {
  it("reads the map's own count after a click and closes onto the list", async () => {
    const store = storeWith(['countries', 'br'], ['countries', 'mx']);
    render(JobsMapDialog, { open: true, store });
    expect(await screen.findByRole('button', { name: 'Show 1,000 jobs' })).toBeTruthy();
    loadCityTable.mockResolvedValue(null);
    await fireEvent.click(screen.getByRole('button', { name: 'Brazil: 700 jobs' }));
    const show = await screen.findByRole('button', { name: 'Show 700 jobs' });
    await fireEvent.click(show);
    await waitFor(() => expect(screen.queryByRole('button', { name: /^Show/ })).toBeNull());
  });

  it('never offers the count from before the last click while the map updates', async () => {
    const store = storeWith(['countries', 'br'], ['countries', 'mx']);
    render(JobsMapDialog, { open: true, store });
    expect(await screen.findByRole('button', { name: 'Show 1,000 jobs' })).toBeTruthy();
    loadCityTable.mockResolvedValue(null);
    facetCounts.mockImplementation(() => new Promise(() => {}));
    await fireEvent.click(screen.getByRole('button', { name: 'Brazil: 700 jobs' }));
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Show 1,000 jobs' })).toBeNull());
  });

  it('lets a newer response win over an older one still in flight', async () => {
    const resolvers: ((c: FacetCounts) => void)[] = [];
    const store = storeWith(['countries', 'br'], ['countries', 'mx'], ['countries', 'us']);
    render(JobsMapDialog, { open: true, store });
    await screen.findByRole('button', { name: 'Mexico: 250 jobs' });

    facetCounts.mockImplementation(() => new Promise((resolve) => resolvers.push(resolve)));
    await fireEvent.click(screen.getByRole('button', { name: 'Mexico: 250 jobs' }));
    await waitFor(() => expect(resolvers).toHaveLength(1));
    store.clearFacet('countries');
    store.add('countries', 'us');
    await waitFor(() => expect(resolvers).toHaveLength(2));

    resolvers[1]?.(counts(50, { countries: { us: 50 }, cities: {}, regions: {} }));
    expect(await screen.findByRole('button', { name: 'Show 50 jobs' })).toBeTruthy();
    resolvers[0]?.(counts(250, { countries: { mx: 250 }, cities: {}, regions: {} }));
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.getByRole('button', { name: 'Show 50 jobs' })).toBeTruthy();
    expect(screen.queryByText('Updating…')).toBeNull();
  });

  it('keeps showing it is updating while a newer request is outstanding', async () => {
    const resolvers: ((c: FacetCounts) => void)[] = [];
    const store = storeWith(['countries', 'br'], ['countries', 'mx']);
    render(JobsMapDialog, { open: true, store });
    await screen.findByRole('button', { name: 'Mexico: 250 jobs' });

    facetCounts.mockImplementation(() => new Promise((resolve) => resolvers.push(resolve)));
    store.add('countries', 'us');
    await waitFor(() => expect(resolvers).toHaveLength(1));
    store.remove('countries', 'us');
    await waitFor(() => expect(resolvers).toHaveLength(2));
    resolvers[0]?.(WORLD);
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.getByText('Updating…')).toBeTruthy();
  });
});

describe('JobsMapDialog — Russian', () => {
  it('words and formats its counts for the locale', async () => {
    page.data = { locale: 'ru' };
    render(JobsMapDialog, { open: true, store: storeWith() });
    const n = (1_000).toLocaleString('ru');
    expect(await screen.findByRole('button', { name: `Показать ${n} вакансий` })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Удалённо из любой страны · 40' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Brazil: 700 вакансий' })).toBeTruthy();
  });
});
