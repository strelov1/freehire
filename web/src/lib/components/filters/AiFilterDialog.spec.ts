import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { CatalogScale, FacetCounts, Interpretation, ResumeProfile } from '#lib/types.js';
import type { FilterStore } from '#lib/filters.js';
import { planParams } from '#lib/aiFilter.js';
import AiFilterDialog from './AiFilterDialog.svelte';
import AiFilterButton from './AiFilterButton.svelte';

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/jobs') } }));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), invalidateAll: vi.fn() }));

const { interpretSearch, facetCounts, catalogScale, extractResumeProfile, authed } = vi.hoisted(() => ({
  interpretSearch: vi.fn<(text: string, previous?: Interpretation) => Promise<Interpretation>>(),
  facetCounts: vi.fn<(params: URLSearchParams, opts?: { facets?: string[] }) => Promise<FacetCounts>>(),
  catalogScale: vi.fn<() => Promise<CatalogScale>>(),
  extractResumeProfile: vi.fn<(file: File) => Promise<ResumeProfile>>(),
  authed: { value: false },
}));

vi.mock('#lib/api.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('#lib/api.js')>();
  return { ...actual, api: { ...actual.api, interpretSearch, facetCounts, catalogScale, extractResumeProfile } };
});

vi.mock('#lib/auth.svelte.js', () => ({
  isAuthenticated: () => authed.value,
  currentUser: () => (authed.value ? { tier: 'free' } : null),
}));

const BRAZIL: Interpretation = {
  summary: 'Product designer roles in Brazil, not in the United States.',
  facets: { category: ['design'], countries: ['br'] },
  exclude: { countries: ['us'] },
  query: 'product designer',
  unresolved: ['vibes'],
  empty: false,
};

const EMPTY: Interpretation = {
  summary: '',
  facets: {},
  exclude: {},
  query: '',
  unresolved: ['test'],
  empty: true,
};

function store() {
  return { apply: vi.fn() } as unknown as FilterStore & { apply: ReturnType<typeof vi.fn> };
}

async function describeSearch(text: string) {
  const field = screen.getByRole('textbox', { name: 'Describe the jobs you want' });
  await fireEvent.input(field, { target: { value: text } });
  await fireEvent.keyDown(field, { key: 'Enter' });
}

beforeEach(() => {
  authed.value = false;
  interpretSearch.mockReset().mockResolvedValue(BRAZIL);
  facetCounts.mockReset().mockResolvedValue({
    total: 799,
    facets: { countries: { br: 700, mx: 99 } },
    stats: {},
    companies_total: 212,
    top_companies: [{ slug: 'acme', name: 'Acme', count: 40 }],
  });
  catalogScale.mockReset().mockResolvedValue({
    open_jobs: 2_000_000,
    unique_open_jobs: 1_500_000,
    companies: 1,
    sources: 1,
    ats_platforms: 1,
    telegram_channels: 1,
    computed_at: '',
    exact: true,
  });
  extractResumeProfile.mockReset();
});

describe('AiFilterButton', () => {
  it('opens the AI search for a signed-out visitor', async () => {
    render(AiFilterButton, { store: store() });

    await fireEvent.click(screen.getByRole('button', { name: /Describe with AI/ }));

    expect(screen.getByRole('dialog', { name: 'AI search' })).toBeTruthy();
  });
});

describe('AiFilterDialog', () => {
  it('shows the plan with its applied-filter chips, the exclusion struck through', async () => {
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });

    await describeSearch('Product designer in Brazil');

    await screen.findByText(BRAZIL.summary);
    expect(interpretSearch).toHaveBeenCalledWith('Product designer in Brazil', undefined);
    expect(screen.getByText('Brazil')).toBeTruthy();
    expect(screen.getByText('United States').className).toContain('line-through');
    expect(screen.getByText(/Didn't recognise: vibes/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Run search' })).toBeTruthy();
  });

  it('keeps Run search on the plan after Keep chatting, and focuses the message field', async () => {
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });
    await describeSearch('Product designer in Brazil');
    await screen.findByText(BRAZIL.summary);

    await fireEvent.click(screen.getByRole('button', { name: 'Keep chatting' }));

    expect(screen.getByText(/what should I change/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Run search' })).toBeTruthy();
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole('textbox', { name: 'Message to the assistant' })),
    );
  });

  it('sends a refinement with the previous plan and clears the field', async () => {
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });
    await describeSearch('Product designer in Brazil');
    await screen.findByText(BRAZIL.summary);
    const remote = { ...BRAZIL, summary: 'Remote product designer roles in Brazil.' };
    interpretSearch.mockResolvedValueOnce(remote);

    const field = screen.getByRole('textbox', { name: 'Message to the assistant' }) as HTMLTextAreaElement;
    await fireEvent.input(field, { target: { value: 'remote only' } });
    await fireEvent.keyDown(field, { key: 'Enter' });

    await screen.findByText(remote.summary);
    expect(interpretSearch).toHaveBeenLastCalledWith('remote only', BRAZIL);
    expect(field.value).toBe('');
  });

  it('answers an empty interpretation with Start over and no plan card', async () => {
    interpretSearch.mockResolvedValue(EMPTY);
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });

    await describeSearch('test');

    await screen.findByText(/couldn't turn that into a search/);
    expect(screen.queryByRole('button', { name: 'Run search' })).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Start over' }));
    expect(screen.getByRole('textbox', { name: 'Describe the jobs you want' })).toBeTruthy();
  });

  it('invites a guest over the hourly limit to sign in', async () => {
    const { ApiError } = await import('#lib/api.js');
    interpretSearch.mockRejectedValue(new ApiError(429, 'too many requests'));
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });

    await describeSearch('Product designer in Brazil');

    expect(await screen.findByRole('button', { name: 'Sign in' })).toBeTruthy();
    expect(screen.queryByRole('status')).toBeNull();
  });

  it('submits "Find jobs that fit my CV" with the CV context when only a CV is attached', async () => {
    extractResumeProfile.mockResolvedValue({ skills: ['figma', 'ux'], categories: ['design'], seniority: 'senior' });
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    Object.defineProperty(input, 'files', { value: [new File(['%PDF'], 'cv.pdf', { type: 'application/pdf' })] });
    await fireEvent.change(input);
    await screen.findByText(/2 skills found/);

    await describeSearch('');

    await waitFor(() => expect(interpretSearch).toHaveBeenCalled());
    const text = interpretSearch.mock.calls[0]?.[0];
    expect(text).toMatch(/^Find jobs that fit my CV\n\nFrom my CV: seniority senior; field design; skills figma, ux\.$/);
  });

  it('runs the overview and applies the plan with Show N jobs', async () => {
    const s = store();
    const onclose = vi.fn();
    const onApplied = vi.fn();
    render(AiFilterDialog, { store: s, onclose, onApplied });
    await describeSearch('Product designer in Brazil');
    await screen.findByText(BRAZIL.summary);

    await fireEvent.click(screen.getByRole('button', { name: 'Run search' }));

    const show = await screen.findByRole('button', { name: 'Show 799 jobs' });
    expect(facetCounts).toHaveBeenCalledWith(planParams(BRAZIL), { facets: ['countries', 'company_slug'] });
    expect(screen.getByText('1,500,000')).toBeTruthy();
    expect(screen.getByText('212')).toBeTruthy();
    await fireEvent.click(show);
    expect(s.apply).toHaveBeenCalledWith(planParams(BRAZIL).toString());
    expect(onclose).toHaveBeenCalled();
    expect(onApplied).toHaveBeenCalled();
  });

  it('still shows the overview when the company count is refused', async () => {
    const { ApiError } = await import('#lib/api.js');
    facetCounts.mockImplementation(async (_params, opts) => {
      if (opts?.facets?.includes('company_slug')) throw new ApiError(400, 'bad request');
      return { total: 12, facets: { countries: { br: 12 } }, stats: {} };
    });
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });
    await describeSearch('Product designer in Brazil');
    await screen.findByText(BRAZIL.summary);

    await fireEvent.click(screen.getByRole('button', { name: 'Run search' }));

    expect(await screen.findByRole('button', { name: 'Show 12 jobs' })).toBeTruthy();
    expect(screen.getByText('Not available')).toBeTruthy();
  });

  it('re-runs the overview when a refinement is sent from Results', async () => {
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });
    await describeSearch('Product designer in Brazil');
    await screen.findByText(BRAZIL.summary);
    await fireEvent.click(screen.getByRole('button', { name: 'Run search' }));
    await screen.findByRole('button', { name: 'Show 799 jobs' });
    const remote = { ...BRAZIL, summary: 'Remote product designer roles in Brazil.' };
    interpretSearch.mockResolvedValueOnce(remote);
    facetCounts.mockResolvedValueOnce({ total: 120, facets: { countries: { br: 120 } }, stats: {}, companies_total: 30, top_companies: [] });

    const field = screen.getByRole('textbox', { name: 'Message to the assistant' });
    await fireEvent.input(field, { target: { value: 'remote only' } });
    await fireEvent.keyDown(field, { key: 'Enter' });

    expect(await screen.findByRole('button', { name: 'Show 120 jobs' })).toBeTruthy();
    expect(facetCounts).toHaveBeenLastCalledWith(planParams(remote), { facets: ['countries', 'company_slug'] });
  });

  it('states a failed run in the chat and stops thinking', async () => {
    facetCounts.mockRejectedValue(new Error('boom'));
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });
    await describeSearch('Product designer in Brazil');
    await screen.findByText(BRAZIL.summary);

    await fireEvent.click(screen.getByRole('button', { name: 'Run search' }));

    expect(await screen.findByText('boom')).toBeTruthy();
    await waitFor(() => expect(screen.queryByRole('status')).toBeNull());
  });

  it('returns to the plan when the first run fails, so the error and Run search stay in reach', async () => {
    facetCounts.mockRejectedValueOnce(new Error('boom'));
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });
    await describeSearch('Product designer in Brazil');
    await screen.findByText(BRAZIL.summary);

    await fireEvent.click(screen.getByRole('button', { name: 'Run search' }));

    expect(await screen.findByText('boom')).toBeTruthy();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Run search' })).toBeTruthy());
    expect(screen.queryByRole('button', { name: /^Show / })).toBeNull();
  });

  it('drops an answer that arrives after Start over', async () => {
    interpretSearch.mockResolvedValueOnce(EMPTY);
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });
    await describeSearch('test');
    await screen.findByText(/couldn't turn that into a search/);

    let answer!: (r: Interpretation) => void;
    interpretSearch.mockReturnValueOnce(new Promise((r) => (answer = r)));
    const field = screen.getByRole('textbox', { name: 'Message to the assistant' });
    await fireEvent.input(field, { target: { value: 'product designer' } });
    await fireEvent.keyDown(field, { key: 'Enter' });
    await fireEvent.click(screen.getByRole('button', { name: 'Start over' }));
    answer(BRAZIL);
    await new Promise((r) => setTimeout(r));

    interpretSearch.mockResolvedValueOnce(BRAZIL);
    await describeSearch('Data engineer');
    await waitFor(() => expect(interpretSearch).toHaveBeenLastCalledWith('Data engineer', undefined));
  });

  it('sends the CV again when the first interpretation failed', async () => {
    extractResumeProfile.mockResolvedValue({ skills: ['figma'], categories: [], seniority: 'senior' });
    interpretSearch.mockRejectedValueOnce(new Error('gateway down'));
    render(AiFilterDialog, { store: store(), onclose: vi.fn() });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    Object.defineProperty(input, 'files', { value: [new File(['%PDF'], 'cv.pdf', { type: 'application/pdf' })] });
    await fireEvent.change(input);
    await screen.findByText(/1 skill found/);
    await describeSearch('Product designer');
    await screen.findByText('gateway down');

    const field = screen.getByRole('textbox', { name: 'Message to the assistant' });
    await fireEvent.input(field, { target: { value: 'Product designer' } });
    await fireEvent.keyDown(field, { key: 'Enter' });

    await waitFor(() => expect(interpretSearch).toHaveBeenCalledTimes(2));
    expect(interpretSearch.mock.calls[1]?.[0]).toMatch(/From my CV: seniority senior; skills figma\.$/);
  });
});
