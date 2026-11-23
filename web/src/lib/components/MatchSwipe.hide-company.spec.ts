import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';

const meFeed = vi.fn();
const saveJob = vi.fn();
const avoidCompany = vi.fn();
vi.mock('#lib/profile.svelte.js', () => ({ profileStore: { avoidCompany } }));
vi.mock('#lib/api.js', () => ({
  api: {
    meFeed: (...args: unknown[]) => meFeed(...args),
    saveJob: (...args: unknown[]) => saveJob(...args),
    listSavedSearches: vi.fn().mockResolvedValue([]),
    markFeedSeen: vi.fn(),
  },
}));
vi.mock('#lib/analytics.js', () => ({ track: vi.fn() }));
vi.mock('#lib/announce.svelte.js', () => ({ announce: vi.fn() }));
vi.mock('$app/paths', () => ({
  resolve: (path: string, p?: Record<string, string>) => path.replace(/\[(\w+)\]/g, (_, k) => p?.[k] ?? ''),
}));
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/matches') } }));

const { default: MatchSwipe } = await import('./MatchSwipe.svelte');

describe('MatchSwipe hide company', () => {
  const job = (slug: string, company_slug = 'acme') => ({
    public_slug: slug,
    title: slug.toUpperCase(),
    company: 'Acme',
    company_slug,
    skills: [] as string[],
    match: { why: [], may_not_fit: [], unknown: [] },
  });
  const openDialog = async () => {
    const trigger = await screen.findByLabelText('More actions for Acme');
    await fireEvent.click(trigger);
    await fireEvent.click(await screen.findByRole('menuitem', { name: 'Hide company' }));
    await screen.findByText('Hide Acme from Matches?');
    return trigger;
  };
  beforeEach(() => {
    avoidCompany.mockReset();
    meFeed.mockReset();
    saveJob.mockReset().mockResolvedValue(undefined);
  });

  it('cancel keeps the card and returns focus to the menu button', async () => {
    meFeed.mockResolvedValue({ data: [job('a-job')] });
    render(MatchSwipe, { onClose: () => {} });
    const trigger = await openDialog();
    await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await vi.waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(avoidCompany).not.toHaveBeenCalled();
    expect(meFeed).toHaveBeenCalledTimes(1);
    expect(screen.getByLabelText('Job: A-JOB at Acme')).toBeTruthy();
  });

  it('explains a full list (409) and retains the card', async () => {
    meFeed.mockResolvedValue({ data: [job('a-job')] });
    avoidCompany.mockRejectedValue(new ApiError(409, 'full'));
    render(MatchSwipe, { onClose: () => {} });
    await openDialog();
    await fireEvent.click(screen.getByRole('button', { name: 'Hide company' }));
    expect((await screen.findByRole('alert')).textContent).toContain('list is full');
    expect(screen.getByLabelText('Job: A-JOB at Acme')).toBeTruthy();
    expect(meFeed).toHaveBeenCalledTimes(1);
  });

  it('reloads the feed, drops stale cards and clears Undo after a successful hide', async () => {
    meFeed.mockResolvedValueOnce({ data: [job('a-job'), job('b-job')] });
    render(MatchSwipe, { onClose: () => {} });
    await fireEvent.click(await screen.findByLabelText('Save'));
    await vi.waitFor(() => expect(screen.getByLabelText('Job: B-JOB at Acme')).toBeTruthy());
    expect((screen.getByLabelText('Undo last') as HTMLButtonElement).disabled).toBe(false);

    avoidCompany.mockResolvedValue({});
    meFeed.mockResolvedValueOnce({ data: [job('c-job', 'other')] });
    await openDialog();
    await fireEvent.click(screen.getByRole('button', { name: 'Hide company' }));
    await vi.waitFor(() => expect(screen.getByLabelText('Job: C-JOB at Acme')).toBeTruthy());
    expect(avoidCompany).toHaveBeenCalledWith('acme');
    expect(screen.queryByLabelText('Job: B-JOB at Acme')).toBeNull();
    expect((screen.getByLabelText('Undo last') as HTMLButtonElement).disabled).toBe(true);
  });

  it('offers no action for a card without a company slug', async () => {
    meFeed.mockResolvedValue({ data: [job('a-job', '')] });
    render(MatchSwipe, { onClose: () => {} });
    await fireEvent.click(await screen.findByLabelText('More actions for Acme'));
    await screen.findByRole('menuitem', { name: 'Already applied' });
    expect(screen.queryByRole('menuitem', { name: 'Hide company' })).toBeNull();
  });
});
